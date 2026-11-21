package probe

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Status classifies validator health for alerting, upgrades, and monthly SLO ETL.
type Status string

const (
	StatusHealthy     Status = "healthy"
	StatusDegraded    Status = "degraded"
	StatusStalled     Status = "stalled"
	StatusUnreachable Status = "unreachable"
)

// Target defines a Substrate validator RPC + Prometheus metrics endpoint pair.
type Target struct {
	Name           string `json:"name"`
	OperatorType   string `json:"operator_type"` // "fno" or "spo"
	Cloud          string `json:"cloud"`         // "aws", "gcp", or "bare-metal"
	Region         string `json:"region"`
	RPCURL         string `json:"rpc_url"`
	MetricsURL     string `json:"metrics_url"`
	MinPeers       int    `json:"min_peers"`
	MaxFinalityLag uint64 `json:"max_finality_lag"`
}

// Snapshot captures a single synchronized observation of a Substrate validator.
type Snapshot struct {
	Target          Target    `json:"target"`
	Timestamp       time.Time `json:"timestamp"`
	Reachable       bool      `json:"reachable"`
	Status          Status    `json:"status"`
	Reason          string    `json:"reason,omitempty"`
	IsSyncing       bool      `json:"is_syncing"`
	RPCPeers        int       `json:"rpc_peers"`
	RPCHeaderHeight uint64    `json:"rpc_header_height"`
	BestHeight      uint64    `json:"best_height"`
	FinalizedHeight uint64    `json:"finalized_height"`
	FinalityLag     uint64    `json:"finality_lag"`
	Libp2pPeers     int       `json:"libp2p_peers"`
	BlocksProduced  uint64    `json:"blocks_produced"`
	LatencyMS       int64     `json:"latency_ms"`
	Error           string    `json:"error,omitempty"`
}

// Prober queries Substrate JSON-RPC and Prometheus metrics endpoints concurrently.
type Prober struct {
	client *http.Client
}

// NewProber constructs a Prober with the provided per-request HTTP timeout.
func NewProber(timeout time.Duration) *Prober {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Prober{
		client: &http.Client{Timeout: timeout},
	}
}

type systemHealthResponse struct {
	Peers           int  `json:"peers"`
	IsSyncing       bool `json:"isSyncing"`
	ShouldHavePeers bool `json:"shouldHavePeers"`
}

type chainHeaderResponse struct {
	Number string `json:"number"`
}

// ProbeAll collects snapshots for every validator target concurrently.
func (p *Prober) ProbeAll(ctx context.Context, targets []Target) []Snapshot {
	out := make([]Snapshot, len(targets))
	var wg sync.WaitGroup
	for i, tgt := range targets {
		wg.Add(1)
		go func(idx int, t Target) {
			defer wg.Done()
			out[idx] = p.Probe(ctx, t)
		}(i, tgt)
	}
	wg.Wait()
	return out
}

// Probe queries JSON-RPC system_health, chain_getHeader, and Prometheus /metrics for a single validator.
func (p *Prober) Probe(ctx context.Context, target Target) Snapshot {
	if target.MinPeers <= 0 {
		target.MinPeers = 3
	}
	if target.MaxFinalityLag == 0 {
		target.MaxFinalityLag = 5
	}

	start := time.Now()
	snap := Snapshot{
		Target:    target,
		Timestamp: start.UTC(),
		Status:    StatusUnreachable,
	}

	var (
		wg        sync.WaitGroup
		health    systemHealthResponse
		headerNum uint64
		metrics   SubstrateMetrics
		healthErr error
		headerErr error
		metricErr error
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		health, healthErr = p.callSystemHealth(ctx, target.RPCURL)
	}()
	go func() {
		defer wg.Done()
		headerNum, headerErr = p.callChainHeader(ctx, target.RPCURL)
	}()
	go func() {
		defer wg.Done()
		metrics, metricErr = p.scrapePrometheus(ctx, target.MetricsURL)
	}()
	wg.Wait()

	snap.LatencyMS = time.Since(start).Milliseconds()

	if err := errors.Join(healthErr, headerErr, metricErr); err != nil {
		snap.Reachable = false
		snap.Status = StatusUnreachable
		snap.Reason = "endpoint probe failed"
		snap.Error = err.Error()
		return snap
	}

	snap.Reachable = true
	snap.IsSyncing = health.IsSyncing
	snap.RPCPeers = health.Peers
	snap.RPCHeaderHeight = headerNum
	snap.BestHeight = metrics.BestHeight
	if snap.BestHeight == 0 && headerNum > 0 {
		snap.BestHeight = headerNum
	}
	snap.FinalizedHeight = metrics.FinalizedHeight
	snap.BlocksProduced = metrics.BlocksProduced
	snap.Libp2pPeers = metrics.Libp2pPeers
	if snap.Libp2pPeers == 0 && health.Peers > 0 {
		snap.Libp2pPeers = health.Peers
	}

	if snap.BestHeight >= snap.FinalizedHeight {
		snap.FinalityLag = snap.BestHeight - snap.FinalizedHeight
	}

	snap.Status, snap.Reason = EvaluateStatus(snap, target.MinPeers, target.MaxFinalityLag)
	return snap
}

// EvaluateStatus classifies validator health from finality lag, sync state, and peer density.
func EvaluateStatus(snap Snapshot, minPeers int, maxLag uint64) (Status, string) {
	if !snap.Reachable {
		return StatusUnreachable, "node unreachable"
	}
	if snap.IsSyncing {
		return StatusStalled, "node is still syncing with peers"
	}
	if snap.FinalityLag > maxLag {
		return StatusStalled, fmt.Sprintf("finality lag %d exceeds max threshold %d", snap.FinalityLag, maxLag)
	}
	peers := snap.Libp2pPeers
	if snap.RPCPeers > peers {
		peers = snap.RPCPeers
	}
	if peers < minPeers {
		return StatusDegraded, fmt.Sprintf("peer count %d below minimum %d", peers, minPeers)
	}
	return StatusHealthy, "ok"
}

func (p *Prober) callJSONRPC(ctx context.Context, rpcURL, method string, result any) error {
	payload := map[string]any{
		"id":      1,
		"jsonrpc": "2.0",
		"method":  method,
		"params":  []any{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s HTTP status %d", method, resp.StatusCode)
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return fmt.Errorf("%s RPC error %d: %s", method, envelope.Error.Code, envelope.Error.Message)
	}
	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return fmt.Errorf("%s returned empty result", method)
	}
	return json.Unmarshal(envelope.Result, result)
}

func (p *Prober) callSystemHealth(ctx context.Context, rpcURL string) (systemHealthResponse, error) {
	var out systemHealthResponse
	err := p.callJSONRPC(ctx, rpcURL, "system_health", &out)
	return out, err
}

func (p *Prober) callChainHeader(ctx context.Context, rpcURL string) (uint64, error) {
	var hdr chainHeaderResponse
	if err := p.callJSONRPC(ctx, rpcURL, "chain_getHeader", &hdr); err != nil {
		return 0, err
	}
	if hdr.Number == "" {
		return 0, errors.New("chain_getHeader missing block number")
	}
	trimmed := strings.TrimPrefix(strings.ToLower(hdr.Number), "0x")
	return strconv.ParseUint(trimmed, 16, 64)
}

// SubstrateMetrics holds core Prometheus metrics exposed by substrate/substrate-validator on :9615.
type SubstrateMetrics struct {
	BestHeight      uint64
	FinalizedHeight uint64
	Libp2pPeers     int
	BlocksProduced  uint64
}

func (p *Prober) scrapePrometheus(ctx context.Context, metricsURL string) (SubstrateMetrics, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return SubstrateMetrics{}, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return SubstrateMetrics{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return SubstrateMetrics{}, fmt.Errorf("metrics HTTP status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return SubstrateMetrics{}, err
	}
	return ParsePrometheusText(string(raw))
}

// ParsePrometheusText extracts Substrate block heights, peer counts, and produced blocks from /metrics text.
func ParsePrometheusText(body string) (SubstrateMetrics, error) {
	var m SubstrateMetrics
	foundAny := false
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		metricToken := fields[0]
		valStr := fields[1]
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		switch {
		case strings.HasPrefix(metricToken, "substrate_block_height"):
			if strings.Contains(metricToken, `status="best"`) {
				m.BestHeight = uint64(val)
				foundAny = true
			} else if strings.Contains(metricToken, `status="finalized"`) {
				m.FinalizedHeight = uint64(val)
				foundAny = true
			}
		case strings.HasPrefix(metricToken, "substrate_sub_libp2p_peers_count"):
			m.Libp2pPeers = int(val)
			foundAny = true
		case strings.HasPrefix(metricToken, "substrate_proposer_block_constructed_count"):
			m.BlocksProduced = uint64(val)
			foundAny = true
		}
	}
	if err := scanner.Err(); err != nil {
		return m, err
	}
	if !foundAny {
		return m, errors.New("no substrate validator metrics matched in /metrics output")
	}
	return m, nil
}

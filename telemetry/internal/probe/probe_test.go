package probe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func mockValidatorServers(t *testing.T, peers int, isSyncing bool, headerHex string, promBody string, forceRPCStatus int) (*httptest.Server, *httptest.Server) {
	t.Helper()
	rpcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if forceRPCStatus != 0 {
			http.Error(w, "upstream error", forceRPCStatus)
			return
		}
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "system_health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"peers":           peers,
					"isSyncing":       isSyncing,
					"shouldHavePeers": true,
				},
			})
		case "chain_getHeader":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"result": map[string]any{
					"number": headerHex,
				},
			})
		default:
			http.Error(w, "unknown method", http.StatusNotFound)
		}
	}))

	metricsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(promBody))
	}))

	t.Cleanup(func() {
		rpcSrv.Close()
		metricsSrv.Close()
	})
	return rpcSrv, metricsSrv
}

func TestParsePrometheusText(t *testing.T) {
	raw := `
# HELP substrate_block_height Current block height
substrate_block_height{status="best",chain="midnight_mainnet"} 120050
substrate_block_height{status="finalized",chain="midnight_mainnet"} 120048
substrate_sub_libp2p_peers_count{chain="midnight_mainnet"} 42
substrate_proposer_block_constructed_count{chain="midnight_mainnet"} 819
`
	m, err := ParsePrometheusText(raw)
	if err != nil {
		t.Fatalf("ParsePrometheusText returned unexpected error: %v", err)
	}
	if m.BestHeight != 120050 || m.FinalizedHeight != 120048 {
		t.Fatalf("unexpected heights: %+v", m)
	}
	if m.Libp2pPeers != 42 || m.BlocksProduced != 819 {
		t.Fatalf("unexpected peers/blocks: %+v", m)
	}
}

func TestProbeScenarios(t *testing.T) {
	ctx := context.Background()
	prober := NewProber(2 * time.Second)

	t.Run("healthy spo validator", func(t *testing.T) {
		prom := `
substrate_block_height{status="best"} 1000
substrate_block_height{status="finalized"} 998
substrate_sub_libp2p_peers_count 36
substrate_proposer_block_constructed_count 250
`
		rpcSrv, metSrv := mockValidatorServers(t, 36, false, "0x3e8", prom, 0)
		snap := prober.Probe(ctx, Target{
			Name:           "spo-eu-aws-01",
			OperatorType:   "spo",
			Cloud:          "aws",
			Region:         "eu-central-1",
			RPCURL:         rpcSrv.URL,
			MetricsURL:     metSrv.URL,
			MinPeers:       12,
			MaxFinalityLag: 5,
		})
		if !snap.Reachable || snap.Status != StatusHealthy {
			t.Fatalf("expected healthy snapshot, got %+v", snap)
		}
		if snap.FinalityLag != 2 || snap.RPCHeaderHeight != 1000 {
			t.Fatalf("expected lag=2 and header=1000, got lag=%d header=%d", snap.FinalityLag, snap.RPCHeaderHeight)
		}
	})

	t.Run("finality stall triggers stalled", func(t *testing.T) {
		prom := `
substrate_block_height{status="best"} 1050
substrate_block_height{status="finalized"} 1020
substrate_sub_libp2p_peers_count 28
`
		rpcSrv, metSrv := mockValidatorServers(t, 28, false, "0x41a", prom, 0)
		snap := prober.Probe(ctx, Target{
			Name:           "fno-gcp-01",
			RPCURL:         rpcSrv.URL,
			MetricsURL:     metSrv.URL,
			MinPeers:       10,
			MaxFinalityLag: 6,
		})
		if snap.Status != StatusStalled || snap.FinalityLag != 30 {
			t.Fatalf("expected stalled with lag 30, got status=%s lag=%d", snap.Status, snap.FinalityLag)
		}
	})

	t.Run("low peers triggers degraded", func(t *testing.T) {
		prom := `
substrate_block_height{status="best"} 500
substrate_block_height{status="finalized"} 499
substrate_sub_libp2p_peers_count 2
`
		rpcSrv, metSrv := mockValidatorServers(t, 2, false, "0x1f4", prom, 0)
		snap := prober.Probe(ctx, Target{
			Name:           "baremetal-01",
			RPCURL:         rpcSrv.URL,
			MetricsURL:     metSrv.URL,
			MinPeers:       8,
			MaxFinalityLag: 5,
		})
		if snap.Status != StatusDegraded {
			t.Fatalf("expected degraded, got %s (%s)", snap.Status, snap.Reason)
		}
	})

	t.Run("syncing node triggers stalled", func(t *testing.T) {
		prom := `
substrate_block_height{status="best"} 500
substrate_block_height{status="finalized"} 500
substrate_sub_libp2p_peers_count 20
`
		rpcSrv, metSrv := mockValidatorServers(t, 20, true, "0x1f4", prom, 0)
		snap := prober.Probe(ctx, Target{
			Name:           "syncing-01",
			RPCURL:         rpcSrv.URL,
			MetricsURL:     metSrv.URL,
			MinPeers:       8,
			MaxFinalityLag: 5,
		})
		if snap.Status != StatusStalled {
			t.Fatalf("expected stalled during sync, got %s", snap.Status)
		}
	})

	t.Run("rpc 500 returns unreachable", func(t *testing.T) {
		prom := `substrate_block_height{status="best"} 10`
		rpcSrv, metSrv := mockValidatorServers(t, 10, false, "0xa", prom, http.StatusInternalServerError)
		snap := prober.Probe(ctx, Target{
			Name:       "down-node",
			RPCURL:     rpcSrv.URL,
			MetricsURL: metSrv.URL,
		})
		if snap.Reachable || snap.Status != StatusUnreachable {
			t.Fatalf("expected unreachable, got %+v", snap)
		}
	})
}

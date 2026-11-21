package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"time"

	"github.com/Kamrank47/midnight-validator-platform/telemetry/internal/etl"
	"github.com/Kamrank47/midnight-validator-platform/telemetry/internal/probe"
)

type configFile struct {
	Network    string         `json:"network"`
	WindowDays int            `json:"window_days"`
	Targets    []probe.Target `json:"targets"`
}

type simSpec struct {
	name      string
	role      string
	cloud     string
	region    string
	peers     int
	best      uint64
	finalized uint64
	blocks    uint64
	uptimeOK  int
	uptimeTot int
	incidents []string
}

func startSimulatedCluster() ([]probe.Target, []etl.OperatorObservation, func()) {
	specs := []simSpec{
		{
			name: "spo-eu-aws-01", role: "spo", cloud: "aws", region: "eu-central-1",
			peers: 42, best: 184220, finalized: 184218, blocks: 2410,
			uptimeOK: 43199, uptimeTot: 43200,
		},
		{
			name: "spo-baremetal-hetzner-02", role: "spo", cloud: "bare-metal", region: "fsn1",
			peers: 38, best: 184220, finalized: 184219, blocks: 2398,
			uptimeOK: 43200, uptimeTot: 43200,
		},
		{
			name: "fno-eu-gcp-01", role: "fno", cloud: "gcp", region: "europe-west1",
			peers: 29, best: 184219, finalized: 184217, blocks: 1944,
			uptimeOK: 43197, uptimeTot: 43200,
		},
		{
			name: "fno-us-aws-02", role: "fno", cloud: "aws", region: "us-east-1",
			peers: 6, best: 184219, finalized: 184211, blocks: 1812,
			uptimeOK: 43189, uptimeTot: 43200,
			incidents: []string{"Finality lag >5 blocks during simulated peer degradation"},
		},
	}

	var servers []*httptest.Server
	targets := make([]probe.Target, 0, len(specs))
	baseObs := make([]etl.OperatorObservation, 0, len(specs))

	for _, s := range specs {
		spec := s
		rpcSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			switch req.Method {
			case "system_health":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": 1,
					"result": map[string]any{"peers": spec.peers, "isSyncing": false, "shouldHavePeers": true},
				})
			case "chain_getHeader":
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": 1,
					"result": map[string]any{"number": fmt.Sprintf("0x%x", spec.best)},
				})
			default:
				http.Error(w, "method not found", http.StatusNotFound)
			}
		}))
		promSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprintf(w, "substrate_block_height{status=\"best\"} %d\n", spec.best)
			fmt.Fprintf(w, "substrate_block_height{status=\"finalized\"} %d\n", spec.finalized)
			fmt.Fprintf(w, "substrate_sub_libp2p_peers_count %d\n", spec.peers)
			fmt.Fprintf(w, "substrate_proposer_block_constructed_count %d\n", spec.blocks)
		}))
		servers = append(servers, rpcSrv, promSrv)

		tgt := probe.Target{
			Name:           spec.name,
			OperatorType:   spec.role,
			Cloud:          spec.cloud,
			Region:         spec.region,
			RPCURL:         rpcSrv.URL,
			MetricsURL:     promSrv.URL,
			MinPeers:       10,
			MaxFinalityLag: 5,
		}
		targets = append(targets, tgt)
		baseObs = append(baseObs, etl.OperatorObservation{
			Target:             tgt,
			UptimeSamplesTotal: spec.uptimeTot,
			UptimeSamplesOK:    spec.uptimeOK,
			FinalitySamples:    []uint64{1, 2, spec.best - spec.finalized},
			PeerSamples:        []int{spec.peers, spec.peers + 2},
			BlocksProduced:     spec.blocks,
			Incidents:          spec.incidents,
		})
	}

	cleanup := func() {
		for _, srv := range servers {
			srv.Close()
		}
	}
	return targets, baseObs, cleanup
}

func main() {
	demoReport := flag.Bool("demo-report", false, "Probe an in-process mock multi-cloud FNO+SPO fleet and print the monthly SLO report")
	configPath := flag.String("config", "", "Path to JSON targets configuration")
	once := flag.Bool("once", false, "Probe configured targets once and exit")
	format := flag.String("format", "md", "Output format for --once/--demo-report: md or json")
	serveAddr := flag.String("serve", "", "HTTP listen address to expose /metrics, /report.json, and /healthz (e.g. :9620)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	prober := probe.NewProber(3 * time.Second)

	if *demoReport {
		targets, obs, cleanup := startSimulatedCluster()
		defer cleanup()
		snaps := prober.ProbeAll(ctx, targets)
		for i, snap := range snaps {
			obs[i].LatestStatus = snap.Status
		}
		report := etl.BuildMonthlyReport("mainnet-sim", 30, obs, time.Now().UTC())
		if *format == "json" {
			raw, err := etl.RenderJSON(report)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(string(raw))
		} else {
			fmt.Print(etl.RenderMarkdown(report))
		}
		return
	}

	if *configPath == "" {
		log.Fatal("either --demo-report or --config <file.json> must be provided")
	}
	rawCfg, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatalf("reading config: %v", err)
	}
	var cfg configFile
	if err := json.Unmarshal(rawCfg, &cfg); err != nil {
		log.Fatalf("parsing config: %v", err)
	}
	if cfg.WindowDays <= 0 {
		cfg.WindowDays = 30
	}

	runCycle := func() ([]probe.Snapshot, etl.MonthlyReport) {
		pCtx, pCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer pCancel()
		snaps := prober.ProbeAll(pCtx, cfg.Targets)
		obs := make([]etl.OperatorObservation, 0, len(snaps))
		for _, s := range snaps {
			ok := 0
			if s.Reachable {
				ok = 1
			}
			obs = append(obs, etl.OperatorObservation{
				Target:             s.Target,
				UptimeSamplesTotal: 1,
				UptimeSamplesOK:    ok,
				FinalitySamples:    []uint64{s.FinalityLag},
				PeerSamples:        []int{s.Libp2pPeers},
				BlocksProduced:     s.BlocksProduced,
				LatestStatus:       s.Status,
			})
		}
		return snaps, etl.BuildMonthlyReport(cfg.Network, cfg.WindowDays, obs, time.Now().UTC())
	}

	if *once || *serveAddr == "" {
		_, report := runCycle()
		if *format == "json" {
			b, err := etl.RenderJSON(report)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(string(b))
		} else {
			fmt.Print(etl.RenderMarkdown(report))
		}
		return
	}

	var (
		mu         sync.RWMutex
		lastSnaps  []probe.Snapshot
		lastReport etl.MonthlyReport
	)
	lastSnaps, lastReport = runCycle()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(etl.RenderPrometheusMetrics(lastSnaps, lastReport)))
	})
	mux.HandleFunc("/report.json", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		b, _ := etl.RenderJSON(lastReport)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			s, rep := runCycle()
			mu.Lock()
			lastSnaps = s
			lastReport = rep
			mu.Unlock()
		}
	}()

	log.Printf("Serving Midnight validator ETL exporter on %s", *serveAddr)
	log.Fatal(http.ListenAndServe(*serveAddr, mux))
}

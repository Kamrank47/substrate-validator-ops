package etl

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Kamrank47/substrate-validator-ops/telemetry/internal/probe"
)

const SLOTargetPercent = 99.99

// OperatorObservation aggregates telemetry series for one FNO or SPO validator over a reporting window.
type OperatorObservation struct {
	Target             probe.Target `json:"target"`
	UptimeSamplesTotal int          `json:"uptime_samples_total"`
	UptimeSamplesOK    int          `json:"uptime_samples_ok"`
	FinalitySamples    []uint64     `json:"finality_samples"`
	PeerSamples        []int        `json:"peer_samples"`
	BlocksProduced     uint64       `json:"blocks_produced"`
	Incidents          []string     `json:"incidents"`
	LatestStatus       probe.Status `json:"latest_status"`
}

// OperatorSummary captures computed 99.99% SLO compliance and finality stats for monthly reporting.
type OperatorSummary struct {
	Name                        string       `json:"name"`
	OperatorType                string       `json:"operator_type"`
	Cloud                       string       `json:"cloud"`
	Region                      string       `json:"region"`
	AvailabilityPercent         float64      `json:"availability_percent"`
	DowntimeMinutes             float64      `json:"downtime_minutes"`
	ErrorBudgetRemainingMinutes float64      `json:"error_budget_remaining_minutes"`
	SLOCompliant                bool         `json:"slo_compliant"`
	MeanFinalityLag             float64      `json:"mean_finality_lag"`
	MinPeers                    int          `json:"min_peers"`
	MeanPeers                   float64      `json:"mean_peers"`
	BlocksProduced              uint64       `json:"blocks_produced"`
	LatestStatus                probe.Status `json:"latest_status"`
	Incidents                   []string     `json:"incidents"`
}

// MonthlyReport represents an FNO/SPO operations telemetry ETL artifact ready for Grafana/Athena/Markdown.
type MonthlyReport struct {
	GeneratedAt             time.Time         `json:"generated_at"`
	Network                 string            `json:"network"`
	WindowDays              int               `json:"window_days"`
	SLOTargetPercent        float64           `json:"slo_target_percent"`
	ErrorBudgetTotalMinutes float64           `json:"error_budget_total_minutes"`
	FleetUptimePercent      float64           `json:"fleet_uptime_percent"`
	CompliantOperators      int               `json:"compliant_operators"`
	TotalOperators          int               `json:"total_operators"`
	Operators               []OperatorSummary `json:"operators"`
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// BuildMonthlyReport transforms raw validator probe observations into a 99.99% SLO compliance summary.
func BuildMonthlyReport(network string, windowDays int, observations []OperatorObservation, now time.Time) MonthlyReport {
	if windowDays <= 0 {
		windowDays = 30
	}
	windowMinutes := float64(windowDays) * 24 * 60
	budgetTotal := round2(windowMinutes * (1.0 - SLOTargetPercent/100.0))

	report := MonthlyReport{
		GeneratedAt:             now.UTC(),
		Network:                 network,
		WindowDays:              windowDays,
		SLOTargetPercent:        SLOTargetPercent,
		ErrorBudgetTotalMinutes: budgetTotal,
		TotalOperators:          len(observations),
		Operators:               make([]OperatorSummary, 0, len(observations)),
	}

	var fleetTotalSamples, fleetOKSamples int
	for _, obs := range observations {
		var availPct float64
		if obs.UptimeSamplesTotal > 0 {
			availPct = (float64(obs.UptimeSamplesOK) / float64(obs.UptimeSamplesTotal)) * 100.0
		}
		downtimeMin := round2(windowMinutes * (1.0 - availPct/100.0))
		budgetRem := round2(budgetTotal - downtimeMin)

		var lagSum uint64
		for _, l := range obs.FinalitySamples {
			lagSum += l
		}
		var meanLag float64
		if len(obs.FinalitySamples) > 0 {
			meanLag = round2(float64(lagSum) / float64(len(obs.FinalitySamples)))
		}

		minPeers := 0
		var peerSum int
		for i, p := range obs.PeerSamples {
			if i == 0 || p < minPeers {
				minPeers = p
			}
			peerSum += p
		}
		var meanPeers float64
		if len(obs.PeerSamples) > 0 {
			meanPeers = round2(float64(peerSum) / float64(len(obs.PeerSamples)))
		}

		compliant := availPct >= SLOTargetPercent
		if compliant {
			report.CompliantOperators++
		}

		fleetTotalSamples += obs.UptimeSamplesTotal
		fleetOKSamples += obs.UptimeSamplesOK

		incidents := obs.Incidents
		if incidents == nil {
			incidents = []string{}
		}

		report.Operators = append(report.Operators, OperatorSummary{
			Name:                        obs.Target.Name,
			OperatorType:                obs.Target.OperatorType,
			Cloud:                       obs.Target.Cloud,
			Region:                      obs.Target.Region,
			AvailabilityPercent:         round4(availPct),
			DowntimeMinutes:             downtimeMin,
			ErrorBudgetRemainingMinutes: budgetRem,
			SLOCompliant:                compliant,
			MeanFinalityLag:             meanLag,
			MinPeers:                    minPeers,
			MeanPeers:                   meanPeers,
			BlocksProduced:              obs.BlocksProduced,
			LatestStatus:                obs.LatestStatus,
			Incidents:                   incidents,
		})
	}

	if fleetTotalSamples > 0 {
		report.FleetUptimePercent = round4((float64(fleetOKSamples) / float64(fleetTotalSamples)) * 100.0)
	}
	return report
}

// RenderJSON serializes the monthly ETL report for S3/Athena or Grafana Infinity ingestion.
func RenderJSON(report MonthlyReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// RenderMarkdown formats an operator-ready monthly FNO/SPO telemetry & SLO report.
func RenderMarkdown(report MonthlyReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Substrate Validator Monthly Operations Report (%s)\n\n", report.Network)
	fmt.Fprintf(&b, "- **Generated:** %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "- **Window:** %d days | **SLO Target:** %.2f%% (Monthly Error Budget: %.2f min)\n",
		report.WindowDays, report.SLOTargetPercent, report.ErrorBudgetTotalMinutes)
	fmt.Fprintf(&b, "- **Fleet Uptime:** %.4f%% | **SLO Compliant Operators:** %d/%d\n\n",
		report.FleetUptimePercent, report.CompliantOperators, report.TotalOperators)

	b.WriteString("| Validator | Role | Cloud | Region | Availability | Error Budget Left | Mean Lag | Min/Mean Peers | Blocks | Status | SLO |\n")
	b.WriteString("|---|---|---|---|---:|---:|---:|---|---:|---|---|\n")
	for _, op := range report.Operators {
		sloBadge := "PASS"
		if !op.SLOCompliant {
			sloBadge = "BREACH"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %.4f%% | %.2f min | %.2f | %d / %.1f | %d | %s | %s |\n",
			op.Name,
			strings.ToUpper(op.OperatorType),
			op.Cloud,
			op.Region,
			op.AvailabilityPercent,
			op.ErrorBudgetRemainingMinutes,
			op.MeanFinalityLag,
			op.MinPeers,
			op.MeanPeers,
			op.BlocksProduced,
			op.LatestStatus,
			sloBadge,
		)
	}
	return b.String()
}

// RenderPrometheusMetrics exposes live validator probes and SLO budget gauges in Prometheus text format.
func RenderPrometheusMetrics(snapshots []probe.Snapshot, report MonthlyReport) string {
	var b strings.Builder
	b.WriteString("# HELP validator_ops_up Validator reachability (1=up, 0=unreachable)\n")
	b.WriteString("# TYPE validator_ops_up gauge\n")
	for _, s := range snapshots {
		up := 0
		if s.Reachable {
			up = 1
		}
		fmt.Fprintf(&b, "validator_ops_up{validator=%q,operator=%q,cloud=%q,region=%q} %d\n",
			s.Target.Name, s.Target.OperatorType, s.Target.Cloud, s.Target.Region, up)
	}

	b.WriteString("# HELP validator_ops_finality_lag Blocks between best and finalized head\n")
	b.WriteString("# TYPE validator_ops_finality_lag gauge\n")
	for _, s := range snapshots {
		fmt.Fprintf(&b, "validator_ops_finality_lag{validator=%q,operator=%q} %d\n",
			s.Target.Name, s.Target.OperatorType, s.FinalityLag)
	}

	b.WriteString("# HELP validator_ops_peers Active libp2p peers reported by validator\n")
	b.WriteString("# TYPE validator_ops_peers gauge\n")
	for _, s := range snapshots {
		fmt.Fprintf(&b, "validator_ops_peers{validator=%q,operator=%q} %d\n",
			s.Target.Name, s.Target.OperatorType, s.Libp2pPeers)
	}

	b.WriteString("# HELP validator_ops_error_budget_remaining_minutes Monthly 99.99% SLO error budget remaining in minutes\n")
	b.WriteString("# TYPE validator_ops_error_budget_remaining_minutes gauge\n")
	for _, op := range report.Operators {
		fmt.Fprintf(&b, "validator_ops_error_budget_remaining_minutes{validator=%q,operator=%q} %.2f\n",
			op.Name, op.OperatorType, op.ErrorBudgetRemainingMinutes)
	}
	return b.String()
}

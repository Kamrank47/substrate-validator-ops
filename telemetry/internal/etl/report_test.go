package etl

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Kamrank47/midnight-validator-platform/telemetry/internal/probe"
)

func TestBuildMonthlyReportSLO(t *testing.T) {
	now := time.Date(2026, 11, 21, 16, 0, 0, 0, time.UTC)
	obs := []OperatorObservation{
		{
			Target: probe.Target{
				Name:         "spo-mainnet-aws-01",
				OperatorType: "spo",
				Cloud:        "aws",
				Region:       "eu-central-1",
			},
			UptimeSamplesTotal: 43200,
			UptimeSamplesOK:    43198, // 2 minutes downtime < 4.32m 30-day budget
			FinalitySamples:    []uint64{1, 2, 1, 2},
			PeerSamples:        []int{36, 40, 38},
			BlocksProduced:     1420,
			LatestStatus:       probe.StatusHealthy,
		},
		{
			Target: probe.Target{
				Name:         "fno-testnet-gcp-01",
				OperatorType: "fno",
				Cloud:        "gcp",
				Region:       "europe-west1",
			},
			UptimeSamplesTotal: 43200,
			UptimeSamplesOK:    43188, // 12 minutes downtime -> breaches 99.99%
			FinalitySamples:    []uint64{2, 8, 5, 1},
			PeerSamples:        []int{14, 7, 22},
			BlocksProduced:     980,
			Incidents:          []string{"Finality lag spike during peer flap"},
			LatestStatus:       probe.StatusDegraded,
		},
	}

	report := BuildMonthlyReport("mainnet", 30, obs, now)
	if report.TotalOperators != 2 || report.CompliantOperators != 1 {
		t.Fatalf("expected 1/2 SLO compliant operators, got %d/%d", report.CompliantOperators, report.TotalOperators)
	}
	if report.ErrorBudgetTotalMinutes != 4.32 {
		t.Fatalf("expected 4.32 min monthly budget for 30d at 99.99%%, got %.2f", report.ErrorBudgetTotalMinutes)
	}

	aws := report.Operators[0]
	if !aws.SLOCompliant || aws.ErrorBudgetRemainingMinutes != 2.32 {
		t.Fatalf("unexpected AWS SPO summary: %+v", aws)
	}
	gcp := report.Operators[1]
	if gcp.SLOCompliant || gcp.ErrorBudgetRemainingMinutes >= 0 {
		t.Fatalf("expected GCP FNO breach with negative budget remaining, got %+v", gcp)
	}

	md := RenderMarkdown(report)
	if !strings.Contains(md, "spo-mainnet-aws-01") || !strings.Contains(md, "BREACH") || !strings.Contains(md, "PASS") {
		t.Fatalf("markdown report missing expected rows:\n%s", md)
	}

	rawJSON, err := RenderJSON(report)
	if err != nil {
		t.Fatalf("RenderJSON error: %v", err)
	}
	var decoded MonthlyReport
	if err := json.Unmarshal(rawJSON, &decoded); err != nil {
		t.Fatalf("RenderJSON produced invalid JSON: %v", err)
	}
}

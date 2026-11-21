# Blameless Post-Mortem Template

- **Incident ID / Date:** `INC-YYYY-MM-DD-01`
- **Severity & Network:** `SEV-1 / Mainnet SPO (AWS eu-central-1)`
- **Duration & 99.99% SLO Budget Impact:** `X.XX minutes downtime / 4.32 minutes monthly budget`
- **Owner / On-Call Lead:** `Kamran Hashmi`

## Summary
One-paragraph objective overview of customer/validator impact, finality lag, or missed slots.

## Timeline (UTC)
- `HH:MM` Alert `MidnightValidatorFinalizedHeadStalled` fired in Prometheus.
- `HH:MM` On-call confirmed root cause and executed anti-equivocation verification.
- `HH:MM` Mitigation complete; finalized block height resumed advancing.

## Root Cause & Contributing Factors
- Primary technical trigger
- Detection / telemetry / IaC guardrail gap

## Action Items
| Action | Owner | Preventative Control | Due Date |
|---|---|---|---|
| Add OPA Rego / Prometheus alert rule | DevOps | Blocks recurrence in CI / alerts faster | YYYY-MM-DD |

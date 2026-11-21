# Midnight Validator Infrastructure Platform

Production reference platform for running, securing, automating, and observing **Midnight Network** validator infrastructure across **AWS**, **Google Cloud (GCP)**, and **bare-metal Linux**, covering the progression from **Federated Node Operators (FNOs)** to decentralised **Stake Pool Operators (SPOs)** on Cardano's partner-chain architecture.

```
                         Public libp2p P2P (:30333 ONLY)
                                       |
         +-----------------------------+-----------------------------+
         v                                                           v
  [ AWS NLB / EKS ]                                          [ GCP Shielded VM / GKE ]
  KMS EBS + IMDSv2                                           CMEK PD-SSD + OS Login
  Secrets Manager                                            Secret Manager
         |                                                           |
         +-------> Internal Ops / Prometheus Only (:9944 / :9615) <--+
                                       |
                                       v
           [ Go Telemetry & 99.99% SLO ETL (`validator-etl`) ]
           Substrate JSON-RPC + :9615 Metrics -> Prom / Grafana / JSON & MD Reports
                                       |
           +---------------------------+---------------------------+
           v                                                       v
    [ Bare-Metal Rollout ]                               [ CI Guardrails ]
    systemd + Compose + Upgrade Script                   OpenTofu + OPA Conftest + Go -race
```

## Architecture & Capabilities

1. **Multi-Cloud IaC (`terraform/`):**
   - Reusable Terraform / OpenTofu modules for AWS (`terraform/modules/aws-validator`) and GCP (`terraform/modules/gcp-validator`) composed into `envs/testnet` (FNO) and `envs/mainnet` (SPO).
   - Enforces network isolation (`30333` P2P public only via NLB/firewall; `9944` JSON-RPC and `9615` Prometheus restricted to internal VPN/monitoring CIDRs), KMS/CMEK encrypted chain volumes, IMDSv2 (`http_tokens = "required"`), Secrets Manager keystore references, and `max_size = 1` anti-equivocation ASG cardinality.
2. **Kubernetes StatefulSets, FNO → SPO Overlays & Argo CD GitOps (`k8s/`):**
   - Hardened `StatefulSet` (`k8s/base/`) with PVC persistence, non-root `UID 10001`, read-only rootfs, dropped capabilities, `NetworkPolicy`, `PodDisruptionBudget`, and Prometheus `ServiceMonitor`.
   - Kustomize overlays promoting `dev` -> `fno-testnet` -> `spo-mainnet` (adding the `cardano-node` follower IPC sidecar and `1000Gi` mainnet volume), managed via Argo CD (`k8s/argocd/`) with `autoSync: "false"` on `spo-mainnet` to prevent unsafe automated restarts.
3. **Bare-Metal Linux Rollout & Zero-Equivocation Upgrades (`bare-metal/`):**
   - Hardened `systemd` unit (`midnight-node.service`), FNO and SPO Docker Compose stacks (`127.0.0.1` loopback binding for `9944`/`9615`, `node_exporter`), host hardening script (`harden-validator-host.sh` covering UFW, BBR/RocksDB `sysctl`, `chrony` NTP drift check, `0600` keystore perms), and `rolling-upgrade.sh` (sync check, anti-double-signing process guard, pre/post finalized height verification, automatic rollback on stall).
4. **Go Validator Probe, Prometheus Exporter & Monthly SLO ETL (`telemetry/`):**
   - Standard-library Go tool (`cmd/validator-etl`) probing Substrate JSON-RPC (`system_health`, `chain_getHeader`) and Prometheus `/metrics` (`substrate_block_height`, `substrate_sub_libp2p_peers_count`, `substrate_proposer_block_constructed_count`).
   - Tracks the **99.99% monthly uptime SLO** (`4.32 minutes/month` 30-day error budget), exports JSON & Markdown operator telemetry reports, serves Prometheus `/metrics` & `/report.json`, and ships multi-window burn-rate alerts (`telemetry/prometheus/alert-rules.yaml`) plus an importable Grafana dashboard (`telemetry/grafana/midnight-validator-overview.json`).
5. **OPA / Conftest Policy-as-Code Gates (`policy/`) & Reproducible Nix Shell (`flake.nix`):**
   - HCL2 Terraform policies (`policy/terraform_validator.rego`) and Kubernetes policies (`policy/k8s_validator.rego`) plus deliberate insecure fixtures (`terraform/insecure-draft/main.tf`, `k8s/insecure-draft/bad-validator-pod.yaml`) proving the gate blocks unsafe validator deployments.

## Verified Local Results

| Check | Scope | Result |
|---|---|---|
| Go Telemetry & SLO ETL (`go test -race -v ./...`) | `probe` (6 cases/subtests) + `etl` (SLO math, JSON/MD) | **All tests passed** (race-clean) |
| Go In-Process Multi-Cloud Demo (`validator-etl --demo-report`) | AWS SPO, Bare-Metal SPO, GCP FNO, AWS FNO stall sim | **3/4 PASS, 1 BREACH flagged** |
| Terraform OPA Gate (`conftest --parser hcl2`) | `aws-validator`, `gcp-validator`, `testnet`, `mainnet` | **36/36 passed** |
| Terraform Negative Gate (`terraform/insecure-draft/main.tf`) | Public `9944`/`9615`/`22`, unencrypted EBS/PD, IMDSv1, plaintext key | **8/8 violations blocked** |
| Kubernetes OPA Gate (`conftest`) | `k8s/base/*.yaml` | **56/56 passed** |
| Kubernetes Negative Gate (`k8s/insecure-draft/bad-validator-pod.yaml`) | `Deployment` vs `StatefulSet`, root, public `9944`/`9615`, missing probes | **8/8 violations blocked** |

## Quickstart

```bash
# Run the full local Go unit + httptest suite and simulated FNO/SPO monthly SLO report
cd telemetry
go vet ./...
go test -race -v ./...
go run ./cmd/validator-etl --demo-report

# Validate Terraform formatting, scripts, and OPA/Conftest policy gates
cd ..
make fmt policy-check lint-scripts
```

## Operational Documentation

- [Architecture & Anti-Equivocation Controls](docs/architecture.md)
- [Runbook: Finality Stall & Zero-Equivocation Failover](docs/runbooks/finality-stall-and-failover.md)
- [Runbook: FNO & SPO Protocol Upgrade](docs/runbooks/protocol-upgrade-runbook.md)
- [Blameless Post-Mortem Template](docs/postmortem-template.md)

## Honest Scope Note

This repository is an independent open-source infrastructure reference architecture built to demonstrate Midnight validator deployment, multi-cloud hardening, FNO-to-SPO partner-chain readiness, and Go/Prometheus 99.99% SLO observability; telemetry tests validate against local `httptest` Substrate/Prometheus mocks rather than live mainnet signing keys.

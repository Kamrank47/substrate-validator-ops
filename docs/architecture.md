# Substrate & Partner-Chain Validator Architecture

This reference platform codifies multi-cloud (AWS + GCP) and bare-metal operations for Substrate validators, covering both initial **Federated Node Operators (FNOs)** and the decentralised **Stake Pool Operator (SPO)** partner-chain topology.

## Network & Security Boundary Model

| Port | Protocol | Purpose | Allowed Exposure |
|---|---|---|---|
| `30333` | TCP (libp2p) | Block gossip, peer discovery | Public via NLB (`externalTrafficPolicy: Local`) or host firewall |
| `9944` | TCP (HTTP/WS) | Substrate JSON-RPC (`--rpc-methods=Safe`) | Internal ops/WireGuard VPN CIDR or localhost ONLY |
| `9615` | TCP (HTTP) | Substrate Prometheus `/metrics` | Monitoring namespace / Prometheus scraper CIDR ONLY |
| `22` | TCP (SSH) | Host administration | Bastion / VPN CIDR or GCP OS Login ONLY (never `0.0.0.0/0`) |

## Anti-Equivocation (No Double-Signing) Design

Starting two validator instances holding identical Aura/Grandpa session keys simultaneously causes equivocation / slashing risk. Three independent guardrails prevent this across every layer:

1. **IaC Cardinality (`1`)**: AWS ASG sets `min_size = 1`, `max_size = 1`, `desired_capacity = 1`; Kubernetes `StatefulSet` uses `OrderedReady` and `replicas: 1`.
2. **GitOps Sync Gate**: `k8s/argocd/applicationset-validators.yaml` sets `autoSync: "false"` on `spo-mainnet` so human verification precedes production rollout.
3. **Script & Runbook Gate**: `bare-metal/scripts/rolling-upgrade.sh` asserts active `substrate-validator` count `<= 1`, gracefully stops the active process cleanly before launching the replacement, and verifies finalized height advances post-upgrade.

## FNO -> SPO Partner-Chain Progression

- **FNO overlay (`k8s/overlays/fno-testnet`, `bare-metal/docker-compose.fno.yml`)**: Single `substrate-validator` StatefulSet / container optimized for federated validation and telemetry ETL verification.
- **SPO overlay (`k8s/overlays/spo-mainnet`, `bare-metal/docker-compose.spo.yml`)**: Adds the Cardano follower sidecar (`cardano-node` with UNIX IPC socket `/ipc/node.socket`), partner-chain registration parameters (`CARDANO_SECURITY_PARAMETER=2160`), and 1 TiB encrypted chain storage.

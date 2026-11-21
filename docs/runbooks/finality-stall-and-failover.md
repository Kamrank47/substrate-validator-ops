# Runbook: Finality Stall & Zero-Equivocation Validator Failover

**Alerts Covered:** `MidnightValidatorFinalizedHeadStalled`, `MidnightValidatorSLOFastBurn`, `MidnightValidatorFinalityLagHigh`

**SLO Context:** Maintaining **99.99%** monthly availability leaves only **4.32 minutes/month** of total downtime error budget. Never rush a failover at the expense of double-signing.

---

## 1. Immediate Triage (First 60 Seconds)

1. Confirm live node state from an internal ops host:
   ```bash
   curl -sf -H 'Content-Type: application/json' \
     -d '{"id":1,"jsonrpc":"2.0","method":"system_health","params":[]}' http://127.0.0.1:9944
   curl -sf http://127.0.0.1:9615/metrics | grep -E 'substrate_block_height|substrate_sub_libp2p_peers_count'
   ```
2. Run a one-shot telemetry probe across the fleet:
   ```bash
   validator-etl --config telemetry/configs/sample-targets.json --once --format md
   ```
3. Check common root causes before touching failover:
   - Peer count `< 8` -> Inspect AWS NLB target health / GCP firewall on TCP `:30333`.
   - Disk usage `> 80%` -> Expand encrypted gp3 EBS / CMEK PD-SSD chain volume live.
   - NTP offset `> 100ms` -> Verify `chronyc tracking` and force clock sync if drifted.
   - SPO overlay: confirm `cardano-follower` container and `/ipc/node.socket` are healthy.

## 2. Mandatory Anti-Equivocation Checklist Before Cold-Standby Failover

**NEVER start session signing on a standby validator until ALL four checks pass:**

- [ ] Confirm primary `midnight-node` systemd unit / container / pod is **completely terminated** (`systemctl is-active midnight-node` -> `inactive`, `pgrep -xc midnight-node` -> `0`, or K8s pod fully gone — not `Terminating`).
- [ ] Verify network/NLB detachment or node deletion has completed cleanly (wait at least 15s past process termination).
- [ ] Confirm standby node chain database is fully synced (`isSyncing: false`) and Cardano IPC socket is healthy (for SPOs).
- [ ] Promote standby **without** sharing the same session key concurrently, rotate session keys cleanly on-chain afterward if key exposure is suspected, and annotate the promotion in the monthly ETL incident list.

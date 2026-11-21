# Runbook: Protocol & Client Upgrade (FNO & SPO Validators)

1. Validate the target container image digest / binary checksum across `dev` and `fno-testnet` first via Argo CD (`k8s/overlays/fno-testnet`) before promoting to `spo-mainnet`.
2. Ensure recent RocksDB/chain-state snapshot exists on encrypted KMS/CMEK storage.
3. Verify zero active syncing (`isSyncing: false`) and finality lag `<= 5` blocks.
4. Execute clean rolling swap on bare metal:
   ```bash
   ./bare-metal/scripts/rolling-upgrade.sh midnightntwrk/midnight-node@sha256:<verified-digest>
   ```
   Or trigger controlled Argo CD sync on `midnight-spo-mainnet` (`autoSync: "false"` guard ensures operator sign-off).
5. Confirm `substrate_block_height{status="finalized"}` advances within 60–90 seconds and `midnight_validator_up == 1` across Grafana & Prometheus alerts before closing the change window.

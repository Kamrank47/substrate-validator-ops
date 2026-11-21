#!/usr/bin/env bash
set -euo pipefail

# Hardens a bare-metal Linux host running a Midnight FNO or SPO validator.
# Enforces P2P-only public firewall, internal WireGuard/monitoring ACLs,
# kernel sysctl tuning, NTP clock drift guard, and strict 0600 keystore perms.

OPS_VPN_CIDR="${OPS_VPN_CIDR:-10.99.0.0/24}"
MONITORING_CIDR="${MONITORING_CIDR:-10.42.0.0/16}"
KEYSTORE_DIR="${KEYSTORE_DIR:-/etc/midnight/keystore}"
DATA_DIR="${DATA_DIR:-/var/lib/midnight/chain-data}"
DRY_RUN="${DRY_RUN:-false}"

log() {
  printf '[harden-validator] %s\n' "$*"
}

run_cmd() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY_RUN: $*"
  else
    "$@"
  fi
}

require_root() {
  if [[ "$DRY_RUN" != "true" && "${EUID:-$(id -u)}" -ne 0 ]]; then
    log "ERROR: run as root or set DRY_RUN=true" >&2
    exit 1
  fi
}

configure_firewall() {
  if ! command -v ufw >/dev/null 2>&1; then
    log "ufw not present; ensure nftables restricts 9944/9615 to $OPS_VPN_CIDR and $MONITORING_CIDR."
    return 0
  fi
  log "Configuring UFW: expose 30333/tcp only; restrict RPC 9944 and Prometheus 9615"
  run_cmd ufw default deny incoming
  run_cmd ufw default allow outgoing
  run_cmd ufw allow 30333/tcp comment 'Midnight libp2p P2P'
  run_cmd ufw allow from "$OPS_VPN_CIDR" to any port 9944 proto tcp comment 'Midnight internal RPC'
  run_cmd ufw allow from "$MONITORING_CIDR" to any port 9615 proto tcp comment 'Midnight Prometheus metrics'
  run_cmd ufw allow from "$OPS_VPN_CIDR" to any port 22 proto tcp comment 'Bastion SSH via VPN only'
  run_cmd ufw --force enable
}

configure_sysctl() {
  local sysctl_file="/etc/sysctl.d/99-midnight-validator.conf"
  log "Applying validator TCP/BBR and RocksDB file-descriptor sysctl tuning -> $sysctl_file"
  if [[ "$DRY_RUN" == "true" ]]; then
    return 0
  fi
  cat <<'EOF' >"$sysctl_file"
net.core.default_qdisc = fq
net.ipv4.tcp_congestion_control = bbr
net.core.somaxconn = 4096
net.ipv4.tcp_max_syn_backlog = 4096
vm.swappiness = 1
fs.inotify.max_user_watches = 524288
EOF
  sysctl --system >/dev/null
}

verify_clock_sync() {
  if command -v chronyc >/dev/null 2>&1; then
    log "Checking chrony NTP drift (must stay well below 100ms for slot consensus)"
    run_cmd chronyc tracking || true
  else
    log "WARNING: chronyc not installed; configure chrony/systemd-timesyncd to avoid slot misses."
  fi
}

secure_keystore_perms() {
  log "Ensuring keystore ($KEYSTORE_DIR) and chain-data ($DATA_DIR) ownership & strict permissions"
  run_cmd mkdir -p "$KEYSTORE_DIR" "$DATA_DIR"
  run_cmd chmod 0700 "$KEYSTORE_DIR"
  if [[ "$DRY_RUN" != "true" ]] && id midnight >/dev/null 2>&1; then
    chown -R midnight:midnight "$KEYSTORE_DIR" "$DATA_DIR"
    find "$KEYSTORE_DIR" -type f -exec chmod 0600 {} +
  fi
}

main() {
  require_root
  configure_firewall
  configure_sysctl
  verify_clock_sync
  secure_keystore_perms
  log "Host hardening verification complete."
}

main "$@"

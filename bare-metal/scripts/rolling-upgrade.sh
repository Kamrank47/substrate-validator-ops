#!/usr/bin/env bash
set -euo pipefail

# Zero-equivocation rolling upgrade script for bare-metal Substrate validators.
# Verifies chain sync, ensures no duplicate signer exists before swap, records
# pre-upgrade finalized head, rolls container/unit, and verifies finality advancement.

TARGET_IMAGE="${1:-}"
COMPOSE_FILE="${COMPOSE_FILE:-/etc/validator/docker-compose.yml}"
RPC_URL="${RPC_URL:-http://127.0.0.1:9944}"
METRICS_URL="${METRICS_URL:-http://127.0.0.1:9615/metrics}"
ROLLBACK_IMAGE_FILE="/var/lib/validator/.last_good_image"
DRY_RUN="${DRY_RUN:-false}"

log() {
  printf '[rolling-upgrade] %s\n' "$*"
}

die() {
  printf '[rolling-upgrade] ERROR: %s\n' "$*" >&2
  exit 1
}

if [[ -z "$TARGET_IMAGE" ]]; then
  die "Usage: $0 <substrate-validator-image:tag-or-digest>"
fi

check_pre_upgrade_health() {
  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY_RUN: skipping live RPC/metrics query at $RPC_URL"
    return 0
  fi
  local health
  health="$(curl -sf -H 'Content-Type: application/json' \
    -d '{"id":1,"jsonrpc":"2.0","method":"system_health","params":[]}' "$RPC_URL")"
  if echo "$health" | grep -q '"isSyncing":true'; then
    die "Aborting upgrade: validator node is currently syncing ($health)."
  fi
  log "Pre-upgrade health OK: $health"
}

assert_single_signer() {
  # Anti-equivocation check: refuse upgrade if duplicate substrate-validator processes are running.
  local count
  count="$(pgrep -xc substrate-validator 2>/dev/null || true)"
  count="${count:-0}"
  if [[ "$count" -gt 1 ]]; then
    die "Multiple substrate-validator processes detected ($count). Refusing upgrade to prevent equivocation."
  fi
  log "Anti-double-signing check passed (active substrate-validator process count: $count)."
}

get_finalized_height() {
  if [[ "$DRY_RUN" == "true" ]]; then
    echo "1000"
    return 0
  fi
  curl -sf "$METRICS_URL" | awk -F' ' '/substrate_block_height\{.*status="finalized"/ {print $2; exit}'
}

perform_upgrade() {
  local previous_height
  previous_height="$(get_finalized_height)"
  log "Recorded pre-upgrade finalized block height: ${previous_height:-unknown}"

  if [[ "$DRY_RUN" == "true" ]]; then
    log "DRY_RUN: would stop active container cleanly (quiescing validator keys), update image to $TARGET_IMAGE, start compose stack, and verify finalized block advances beyond ${previous_height}"
    return 0
  fi

  local current_image
  current_image="$(docker compose -f "$COMPOSE_FILE" images -q substrate-validator 2>/dev/null || true)"
  if [[ -n "$current_image" ]]; then
    echo "$current_image" >"$ROLLBACK_IMAGE_FILE"
  fi

  # Clean stop before bringing replacement online prevents two instances signing same slot.
  docker compose -f "$COMPOSE_FILE" stop -t 60 substrate-validator
  VALIDATOR_IMAGE="$TARGET_IMAGE" docker compose -f "$COMPOSE_FILE" up -d substrate-validator

  log "Waiting up to 90s for post-upgrade finality to advance..."
  for _ in $(seq 1 18); do
    sleep 5
    local next_height
    next_height="$(get_finalized_height)"
    if [[ -n "$next_height" && -n "$previous_height" && "$next_height" -gt "$previous_height" ]]; then
      log "Upgrade verified: finalized height advanced from $previous_height to $next_height."
      return 0
    fi
  done

  log "ERROR: Finality stalled post-upgrade; triggering rollback."
  if [[ -f "$ROLLBACK_IMAGE_FILE" ]]; then
    docker compose -f "$COMPOSE_FILE" stop -t 60 substrate-validator
    VALIDATOR_IMAGE="$(cat "$ROLLBACK_IMAGE_FILE")" docker compose -f "$COMPOSE_FILE" up -d substrate-validator
  fi
  die "Upgrade rolled back due to post-upgrade finality stall."
}

main() {
  check_pre_upgrade_health
  assert_single_signer
  perform_upgrade
}

main

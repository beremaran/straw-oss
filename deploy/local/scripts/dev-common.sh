#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="${STRAW_ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
COMPOSE_FILE="${STRAW_COMPOSE_FILE:-$ROOT_DIR/deploy/local/docker-compose.yml}"
CONTROL_API_PORT="${STRAW_CONTROL_API_PORT:-8080}"
CONTROL_METRICS_PORT="${STRAW_CONTROL_METRICS_PORT:-9090}"

die() { printf 'local stack: %s\n' "$1" >&2; exit 1; }
require_command() { command -v "$1" >/dev/null 2>&1 || die "missing '$1'; install it and retry"; }
check_prerequisites() {
  require_command docker
  require_command curl
  docker info >/dev/null 2>&1 || die "Docker is unavailable; start Docker and retry"
}
compose() { docker compose -f "$COMPOSE_FILE" "$@"; }
wait_for_url() {
  local url="$1" label="$2" deadline=$((SECONDS + 120))
  while (( SECONDS < deadline )); do
    if curl --fail --silent --show-error --max-time 3 "$url" >/dev/null 2>&1; then return 0; fi
    sleep 2
  done
  die "$label did not become ready within 120 seconds; run 'make dev-status'"
}

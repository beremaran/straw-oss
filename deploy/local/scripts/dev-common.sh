#!/usr/bin/env bash

set -Eeuo pipefail

ROOT_DIR="${STRAW_ROOT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)}"
INFRA_DIR="${STRAW_INFRA_DIR:-$ROOT_DIR/deploy/local}"
DEV_DIR="${STRAW_DEV_DIR:-$INFRA_DIR/.dev}"
LOCAL_ENV="${STRAW_LOCAL_ENV:-$DEV_DIR/local.env}"
LEGACY_ENV="$DEV_DIR/straw-live.env"
COMPOSE_BASE="${STRAW_COMPOSE_BASE:-$INFRA_DIR/docker-compose.yml}"
COMPOSE_LOCAL="${STRAW_COMPOSE_LOCAL:-}"
GIT_ROOT="${STRAW_GIT_ROOT:-$ROOT_DIR}"
TENANT_ID="22222222-2222-4222-8222-222222222222"

die() { printf 'local stack: %s\n' "$1" >&2; exit 1; }
require_command() { command -v "$1" >/dev/null 2>&1 || die "missing '$1'; install it before continuing"; }
check_prerequisites() {
  require_command docker
  require_command curl
  require_command openssl
  docker info >/dev/null 2>&1 || die "Docker is unavailable; start Docker and retry"
}
append_local_env() {
  local name="$1" value="$2"
  printf '%s=%s\n' "$name" "$value" >> "$LOCAL_ENV"
  export "$name=$value"
}
ensure_straw_local_env() {
  mkdir -p "$DEV_DIR"
  if [[ -e "$LOCAL_ENV" ]]; then [[ -f "$LOCAL_ENV" ]] || die "$LOCAL_ENV exists but is not a regular file"; else : > "$LOCAL_ENV"; fi
  chmod 600 "$LOCAL_ENV"
  git -C "$GIT_ROOT" check-ignore -q "$LOCAL_ENV" || die "$LOCAL_ENV is not ignored by git"
  legacy_admin_key=""
  legacy_requester_key=""
  if [[ -f "$LEGACY_ENV" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$LEGACY_ENV"
    set +a
    legacy_admin_key="${STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY:-}"
    legacy_requester_key="${STRAW_API_KEY:-}"
  fi

  set -a
  # shellcheck disable=SC1090
  source "$LOCAL_ENV"
  set +a

  # Older checkouts used straw-live.env. If the new state has not completed
  # requester provisioning, carry that existing local identity forward rather
  # than forcing a destructive reset of healthy volumes.
  if ! grep -q '^STRAW_API_KEY=' "$LOCAL_ENV" && [[ -n "$legacy_requester_key" ]]; then
    if [[ -n "$legacy_admin_key" ]] && ! grep -q '^STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=' "$LOCAL_ENV"; then
      append_local_env STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY "$legacy_admin_key"
    fi
    append_local_env STRAW_API_KEY "$legacy_requester_key"
  fi
  [[ -n "${STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY:-}" ]] || append_local_env STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY "sk_dev_admin_$(openssl rand -hex 24)"
  [[ -n "${STRAW_TENANT_ID:-}" ]] || append_local_env STRAW_TENANT_ID "$TENANT_ID"
  set -a
  # shellcheck disable=SC1090
  source "$LOCAL_ENV"
  set +a
}
compose() {
  local args=(--env-file "$LOCAL_ENV" -f "$COMPOSE_BASE")
  if [[ -n "$COMPOSE_LOCAL" ]]; then args+=(-f "$COMPOSE_LOCAL"); fi
  docker compose "${args[@]}" "$@"
}
wait_for_url() {
  local url="$1" label="$2" deadline=$((SECONDS + 120))
  while (( SECONDS < deadline )); do
    if curl --fail --silent --show-error --max-time 3 "$url" >/dev/null 2>&1; then return 0; fi
    sleep 2
  done
  die "$label did not become ready within 120 seconds; run 'make infra-status' for redacted diagnostics"
}
wait_for_compose_health() {
  local service="$1" label="$2" deadline=$((SECONDS + 120)) container_id health
  while (( SECONDS < deadline )); do
    container_id="$(compose ps -q "$service" 2>/dev/null || true)"
    if [[ -n "$container_id" ]]; then
      health="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
      [[ "$health" == healthy ]] && return 0
    fi
    sleep 2
  done
  die "$label did not become healthy within 120 seconds; run 'make infra-status' for redacted diagnostics"
}

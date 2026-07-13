#!/usr/bin/env bash
set -Eeuo pipefail

profile=${PROFILE:-default}
if [[ ! $profile =~ ^[a-z0-9_-]+$ ]]; then
  echo 'PROFILE must contain only lowercase letters, digits, underscore, or hyphen' >&2
  exit 2
fi

command_line() {
  "$@" 2>/dev/null | head -n 1 || printf 'unavailable'
}

http_status() {
  local status
  status=$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 2 "$1" 2>/dev/null || true)
  [[ $status =~ ^[0-9]{3}$ ]] || status=000
  printf '%s' "$status"
}

# This is deliberately an allowlist, not a log/config sanitizer. Never add raw
# environment, configuration, metrics, logs, request data, or command arguments.
printf 'collected_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
printf 'profile=%s\n' "$profile"
printf 'straw_revision=%s\n' "$(git rev-parse --verify HEAD 2>/dev/null || printf 'unavailable')"
printf 'os_arch=%s\n' "$(command_line uname -sm)"
printf 'go_version=%s\n' "$(command_line go version)"
printf 'docker_client=%s\n' "$(command_line docker version --format '{{.Client.Version}}')"
printf 'compose_version=%s\n' "$(command_line docker compose version --short)"
printf 'control_health_status=%s\n' "$(http_status http://127.0.0.1:${STRAW_CONTROL_METRICS_PORT:-9090}/healthz)"
printf 'control_readiness_status=%s\n' "$(http_status http://127.0.0.1:${STRAW_CONTROL_METRICS_PORT:-9090}/readyz)"

#!/usr/bin/env bash

set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/dev-common.sh"

status=0
printf 'Local stack status\n'
if command -v docker >/dev/null 2>&1; then printf '  %-18s ok\n' docker; else printf '  %-18s missing (install Docker and retry)\n' docker; exit 1; fi
if command -v curl >/dev/null 2>&1; then printf '  %-18s ok\n' curl; else printf '  %-18s missing (install curl and retry)\n' curl; exit 1; fi
if ! docker info >/dev/null 2>&1; then printf '  %-18s unavailable (start Docker and retry)\n' docker-daemon; exit 1; fi

if [[ ! -f "$LOCAL_ENV" ]]; then
  printf '  %-18s absent (run make infra-up)\n' local-credentials
  status=1
else
  chmod 600 "$LOCAL_ENV"
  set -a
  # shellcheck disable=SC1090
  source "$LOCAL_ENV"
  set +a
  if [[ -n "${STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY:-}" ]]; then printf '  %-18s present\n' STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY; else printf '  %-18s missing\n' STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY; status=1; fi
  if [[ -n "${STRAW_API_KEY:-}" ]]; then printf '  %-18s present\n' STRAW_API_KEY; else printf '  %-18s missing\n' STRAW_API_KEY; status=1; fi
  if [[ -n "${STRAW_TENANT_ID:-}" ]]; then printf '  %-18s present\n' STRAW_TENANT_ID; else printf '  %-18s missing\n' STRAW_TENANT_ID; status=1; fi
fi

if curl --fail --silent --max-time 3 http://127.0.0.1:9090/readyz >/dev/null 2>&1; then
  printf '  %-18s ready\n' control
  if [[ -n "${STRAW_API_KEY:-}" ]]; then
    # An empty request body reaches authentication before validation, then
    # returns 400 without dispatching any data-plane work for a valid requester.
    requester_status="$(curl --silent --max-time 3 --output /dev/null --write-out '%{http_code}' \
      -H "Authorization: Bearer ${STRAW_API_KEY}" -H 'Content-Type: application/json' \
      --data '' http://127.0.0.1:8080/api/v1/requests || true)"
    if [[ "$requester_status" == 400 ]]; then
      printf '  %-18s authorized\n' requester-key
    else
      printf '  %-18s stale-or-unauthorized (run make infra-reset, then make infra-up)\n' requester-key
      status=1
    fi
  fi
else
  printf '  %-18s not-ready (run make infra-up; free occupied local ports if it cannot start)\n' control
  status=1
fi
if [[ -f "$LOCAL_ENV" ]] && ! compose ps --format 'table {{.Service}}\t{{.State}}' 2>/dev/null; then
  printf '  %-18s unavailable (run make infra-up)\n' compose
  status=1
fi
exit "$status"

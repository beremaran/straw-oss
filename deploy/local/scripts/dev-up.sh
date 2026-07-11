#!/usr/bin/env bash

set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/dev-common.sh"
temporary_response="$(mktemp "${TMPDIR:-/tmp}/straw-bootstrap.XXXXXX")"
cleanup() { rm -f "$temporary_response"; }
trap cleanup EXIT
check_prerequisites
ensure_straw_local_env
printf 'local stack: starting dependencies and Control...\n'
compose up -d --build control egress >/dev/null 2>&1 || die "Compose could not start Control and Egress; free occupied local ports and run 'make infra-status'"
wait_for_url http://127.0.0.1:9090/readyz "Control"
if [[ -z "${STRAW_API_KEY:-}" ]]; then
  printf 'local stack: provisioning tenant-scoped requester credential...\n'
  curl --fail --silent --show-error --max-time 20 \
    -H "Authorization: Bearer ${STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY}" \
    -H 'Content-Type: application/json' \
    --data '{"role":"requester"}' \
    --output "$temporary_response" \
    "http://127.0.0.1:8080/api/v1/config/tenants/${STRAW_TENANT_ID}/api-keys" \
    || die "Control rejected requester provisioning; run 'make infra-status' or 'make infra-reset' if local state is stale"
  requester_key="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1], encoding="utf-8")).get("secret", ""), end="")' "$temporary_response")"
  [[ -n "$requester_key" ]] || die "Control returned no requester credential; response was not displayed for safety"
  append_local_env STRAW_API_KEY "$requester_key"
fi
wait_for_compose_health control "Control"
printf 'local stack: ready (credentials are stored in %s)\n' "$LOCAL_ENV"

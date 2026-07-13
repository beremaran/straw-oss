#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root"
cleanup() { make dev-down >/dev/null 2>&1 || true; }
trap cleanup EXIT

make dev
response=$(curl --fail --silent --show-error --max-time 30 \
  -H 'Content-Type: application/json' \
  -d '{"method":"GET","url":"https://example.com"}' \
  "http://127.0.0.1:${STRAW_CONTROL_API_PORT:-8080}/api/v1/requests")
case "$response" in
  *'"status":200'*) ;;
  *) echo "quickstart returned unexpected sanitized shape" >&2; exit 1 ;;
esac
curl --fail --silent --show-error "http://127.0.0.1:${STRAW_CONTROL_METRICS_PORT:-9090}/readyz" >/dev/null
echo 'quickstart smoke: passed'

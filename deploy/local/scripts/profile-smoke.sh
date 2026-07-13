#!/usr/bin/env bash
set -Eeuo pipefail

profile=${1:?usage: profile-smoke.sh default|admin|receipts}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root"
api="http://127.0.0.1:${STRAW_CONTROL_API_PORT:-8080}"
metrics="http://127.0.0.1:${STRAW_CONTROL_METRICS_PORT:-9090}"
files=(-f deploy/local/docker-compose.yml)
case "$profile" in
  default) ;;
  admin) files+=(-f deploy/local/docker-compose.runtime-admin.yml) ;;
  receipts) files+=(-f deploy/local/docker-compose.object-storage.yml) ;;
  *) echo "unknown profile: $profile" >&2; exit 2 ;;
esac
compose=(docker compose -p "straw-smoke-${profile}" "${files[@]}")
up_args=(-d --wait)
if [[ ${STRAW_SMOKE_BUILD:-true} == true ]]; then
  up_args+=(--build)
else
  up_args+=(--no-build)
fi
cleanup() { "${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
"${compose[@]}" up "${up_args[@]}"
curl --fail --silent --show-error "$metrics/readyz" >/dev/null
response=$(curl --fail --silent --show-error --max-time 30 -H 'Content-Type: application/json' -d '{"method":"GET","url":"https://example.com"}' "$api/api/v1/requests")
[[ $response == *'"status":200'* ]] || { echo "profile request failed" >&2; exit 1; }

if [[ $profile == admin ]]; then
  headers=$(mktemp)
  curl --fail --silent --show-error -D "$headers" -o /dev/null -H 'Authorization: Bearer local-admin' "$api/api/v1/admin/config"
  revision=$(awk -F '"' 'tolower($1) == "etag: " {print $2}' "$headers")
  rm -f "$headers"
  [[ -n $revision ]]
  curl --fail --silent --show-error -H 'Authorization: Bearer local-admin' "$api/api/v1/admin/workers" | grep -q 'egress-1'
  for action in drain undrain; do
    mutation=$(curl --fail --silent --show-error -H 'Authorization: Bearer local-admin' -H "If-Match: $revision" -X POST "$api/api/v1/admin/workers/egress-1/$action")
    revision=$(printf '%s' "$mutation" | sed -n 's/.*"revision":\([0-9][0-9]*\).*/\1/p')
    [[ -n $revision ]]
  done
fi

if [[ $profile == receipts ]]; then
  created=$(curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"direction":"request","size_bytes":5,"sha256_hex":"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"}' "$api/api/v1/receipts")
  receipt_id=$(printf '%s' "$created" | sed -n 's/.*"receipt_id":"\([^"]*\)".*/\1/p')
  [[ $receipt_id == rcpt_* ]]
  printf hello | curl --fail --silent --show-error -X PUT --data-binary @- "$api/api/v1/receipts/$receipt_id/parts/1" >/dev/null
  curl --fail --silent --show-error -X POST "$api/api/v1/receipts/$receipt_id/complete" | grep -q '"state":"verified"'
  curl --fail --silent --show-error "$api/api/v1/receipts/$receipt_id" | grep -q '"state":"verified"'

  corrupt=$(curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"direction":"request","size_bytes":5,"sha256_hex":"0000000000000000000000000000000000000000000000000000000000000000"}' "$api/api/v1/receipts")
  corrupt_id=$(printf '%s' "$corrupt" | sed -n 's/.*"receipt_id":"\([^"]*\)".*/\1/p')
  [[ $corrupt_id == rcpt_* ]]
  printf hello | curl --fail --silent --show-error -X PUT --data-binary @- "$api/api/v1/receipts/$corrupt_id/parts/1" >/dev/null
  [[ $(curl --silent --output /dev/null --write-out '%{http_code}' -X POST "$api/api/v1/receipts/$corrupt_id/complete") == 400 ]]
  curl --fail --silent --show-error "$api/api/v1/receipts/$corrupt_id" | grep -q '"state":"rejected"'
fi
echo "profile smoke ($profile): passed"

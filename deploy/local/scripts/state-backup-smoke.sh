#!/usr/bin/env bash
set -Eeuo pipefail

profile=${1:?usage: state-backup-smoke.sh admin|receipts}
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
cd "$root"
project="straw-backup-${profile}"
api="http://127.0.0.1:${STRAW_CONTROL_API_PORT:-8080}"
files=(-f deploy/local/docker-compose.yml)
case "$profile" in
  admin) files+=(-f deploy/local/docker-compose.runtime-admin.yml); volume="${project}_straw-runtime-data" ;;
  receipts) files+=(-f deploy/local/docker-compose.object-storage.yml); volume="${project}_straw-object-data" ;;
  *) echo "unknown stateful profile: $profile" >&2; exit 2 ;;
esac
compose=(docker compose -p "$project" "${files[@]}")
up_args=(-d --wait)
if [[ ${STRAW_SMOKE_BUILD:-true} == true ]]; then
  up_args+=(--build)
else
  up_args+=(--no-build)
fi
tmp=$(mktemp -d)
cleanup() { "${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; docker volume rm -f "$volume" >/dev/null 2>&1 || true; rm -rf "$tmp"; }
trap cleanup EXIT

"${compose[@]}" up "${up_args[@]}"
receipt_id=""
if [[ $profile == admin ]]; then
  headers="$tmp/admin-headers"
  curl --fail --silent --show-error -D "$headers" -o /dev/null -H 'Authorization: Bearer local-admin' "$api/api/v1/admin/config"
  revision=$(awk -F '"' 'tolower($1) == "etag: " {print $2}' "$headers")
  [[ -n $revision ]]
  curl --fail --silent --show-error -H 'Authorization: Bearer local-admin' -H "If-Match: $revision" -X POST "$api/api/v1/admin/workers/egress-1/drain" | grep -q '"draining":true'
else
  created=$(curl --fail --silent --show-error -H 'Content-Type: application/json' -d '{"direction":"response","size_bytes":5,"sha256_hex":"2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"}' "$api/api/v1/receipts")
  receipt_id=$(printf '%s' "$created" | sed -n 's/.*"receipt_id":"\([^"]*\)".*/\1/p')
  printf hello | curl --fail --silent --show-error -X PUT --data-binary @- "$api/api/v1/receipts/$receipt_id/parts/1" >/dev/null
  curl --fail --silent --show-error -X POST "$api/api/v1/receipts/$receipt_id/complete" | grep -q '"state":"verified"'
fi

"${compose[@]}" stop
docker run --rm -v "$volume:/data:ro" -v "$tmp:/backup" alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce tar -C /data -czf /backup/state.tgz .
"${compose[@]}" down -v --remove-orphans
docker volume create "$volume" >/dev/null
docker run --rm -v "$volume:/data" -v "$tmp:/backup:ro" alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce tar -C /data -xzf /backup/state.tgz
"${compose[@]}" up "${up_args[@]}"

if [[ $profile == admin ]]; then
  curl --fail --silent --show-error -H 'Authorization: Bearer local-admin' "$api/api/v1/admin/workers" | grep -q '"draining":true'
else
  curl --fail --silent --show-error "$api/api/v1/receipts/$receipt_id" | grep -q '"state":"verified"'
  [[ $(curl --fail --silent --show-error "$api/api/v1/receipts/$receipt_id/content") == hello ]]
fi
echo "state backup/restore smoke ($profile): passed"

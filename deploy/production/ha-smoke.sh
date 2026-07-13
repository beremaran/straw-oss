#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"
project=straw-ha-smoke
env_file=$(mktemp)
printf '%s\n' \
  'STRAW_AUTH_TOKEN=ha-smoke-token' \
  'STRAW_ADMIN_TOKEN=ha-smoke-admin-token' \
  'NATS_USER=straw' \
  'NATS_PASSWORD=ha-smoke-nats-password' \
  'REDIS_PASSWORD=ha-smoke-redis-password' >"$env_file"
compose=(docker compose -p "$project" --env-file "$env_file" -f deploy/production/compose.ha.yml)
up_args=(-d --wait --scale egress=2)
if [[ ${STRAW_SMOKE_BUILD:-true} == true ]]; then
  up_args+=(--build)
else
  up_args+=(--no-build)
fi

cleanup() {
  "${compose[@]}" unpause redis >/dev/null 2>&1 || true
  "${compose[@]}" start control-1 >/dev/null 2>&1 || true
  "${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
  rm -f "$env_file"
}
trap cleanup EXIT

wait_status() {
  local expected=$1 url=$2
  for _ in {1..30}; do
    if [[ $(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 2 "$url" || true) == "$expected" ]]; then
      return 0
    fi
    sleep 1
  done
  echo "timed out waiting for HTTP $expected from $url" >&2
  return 1
}

request() {
  for _ in {1..30}; do
    if curl --fail --silent --max-time 30 \
      -H 'Authorization: Bearer ha-smoke-token' \
      -H 'Content-Type: application/json' \
      -d '{"method":"GET","url":"https://example.com"}' \
      'http://127.0.0.1:8080/api/v1/requests' | grep -q '"status":200'; then
      return 0
    fi
    sleep 1
  done
  echo 'timed out waiting for a successful HA request' >&2
  return 1
}

"${compose[@]}" up "${up_args[@]}"
[[ $("${compose[@]}" ps -q egress | wc -l | tr -d ' ') == 2 ]]
request

"${compose[@]}" stop control-1
request
"${compose[@]}" start control-1
wait_status 200 http://127.0.0.1:9091/readyz

"${compose[@]}" pause redis
wait_status 503 http://127.0.0.1:9091/readyz
wait_status 503 http://127.0.0.1:9092/readyz
"${compose[@]}" unpause redis
wait_status 200 http://127.0.0.1:9091/readyz
wait_status 200 http://127.0.0.1:9092/readyz
request

egress_container=$("${compose[@]}" ps -q egress | head -n 1)
docker stop --time 15 "$egress_container" >/dev/null
request
docker start "$egress_container" >/dev/null

echo 'HA smoke: passed'

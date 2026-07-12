#!/bin/sh
set -eu

tmp_env="$(mktemp)"
trap 'rm -f "$tmp_env"' EXIT

cat >"$tmp_env" <<'ENV'
STRAW_AUTH_TOKEN=example-token
STRAW_ADMIN_TOKEN=example-admin-token
NATS_USER=straw
NATS_PASSWORD=example-password
ENV

docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config -q
docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.runtime-admin.yml config -q

rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config)"
printf '%s\n' "$rendered" | grep -q 'target: 8080'
printf '%s\n' "$rendered" | grep -q 'host_ip: 127.0.0.1'
printf '%s\n' "$rendered" | grep -q 'no-new-privileges:true'

admin_rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.runtime-admin.yml config)"
printf '%s\n' "$admin_rendered" | grep -q 'STRAW_ADMIN_TOKEN'
printf '%s\n' "$admin_rendered" | grep -q 'straw-runtime-data'

go test ./internal/config ./internal/natsx

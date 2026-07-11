#!/bin/sh
set -eu

tmp_env="$(mktemp)"
trap 'rm -f "$tmp_env"' EXIT

cat >"$tmp_env" <<'ENV'
STRAW_AUTH_TOKEN=example-token
NATS_USER=straw
NATS_PASSWORD=example-password
ENV

docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config -q

rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config)"
printf '%s\n' "$rendered" | grep -q 'target: 8080'
printf '%s\n' "$rendered" | grep -q 'host_ip: 127.0.0.1'
printf '%s\n' "$rendered" | grep -q 'no-new-privileges:true'

go test ./internal/config ./internal/natsx

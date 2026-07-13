#!/bin/sh
set -eu

tmp_env="$(mktemp)"
trap 'rm -f "$tmp_env"' EXIT

cat >"$tmp_env" <<'ENV'
STRAW_AUTH_TOKEN=example-token
STRAW_ADMIN_TOKEN=example-admin-token
NATS_USER=straw
NATS_PASSWORD=example-password
REDIS_PASSWORD=example-redis-password
STRAW_RECEIPT_SIGNING_KEY=example-receipt-signing-key-32-bytes
STRAW_S3_ACCESS_KEY=example-access-key
STRAW_S3_SECRET_KEY=example-secret-key
ENV

docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config -q
docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.runtime-admin.yml config -q
docker compose --env-file "$tmp_env" -f deploy/production/compose.ha.yml config -q
docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.object-storage.yml config -q
docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.tls.yml config -q

rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config)"
printf '%s\n' "$rendered" | grep -q 'target: 8080'
printf '%s\n' "$rendered" | grep -q 'host_ip: 127.0.0.1'
printf '%s\n' "$rendered" | grep -q 'no-new-privileges:true'

admin_rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.runtime-admin.yml config)"
printf '%s\n' "$admin_rendered" | grep -q 'STRAW_ADMIN_TOKEN'
printf '%s\n' "$admin_rendered" | grep -q 'straw-runtime-data'

ha_rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.ha.yml config)"
printf '%s\n' "$ha_rendered" | grep -q 'STRAW_CONTROL_INSTANCE_ID: control-1'
printf '%s\n' "$ha_rendered" | grep -q 'STRAW_CONTROL_INSTANCE_ID: control-2'
printf '%s\n' "$ha_rendered" | grep -q 'redis://:'

object_rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.object-storage.yml config)"
printf '%s\n' "$object_rendered" | grep -q 'STRAW_RECEIPT_SIGNING_KEY'
printf '%s\n' "$object_rendered" | grep -q 'control.object-storage.json'

tls_rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml -f deploy/production/compose.tls.yml config)"
printf '%s\n' "$tls_rendered" | grep -q 'target: 8443'
printf '%s\n' "$tls_rendered" | grep -q 'haproxy.tls.cfg'

go test ./internal/config ./internal/natsx

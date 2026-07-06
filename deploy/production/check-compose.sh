#!/bin/sh
set -eu

tmp_env="$(mktemp)"
trap 'rm -f "$tmp_env"' EXIT

cat >"$tmp_env" <<'ENV'
POSTGRES_USER=straw
POSTGRES_PASSWORD=example
POSTGRES_DB=straw
CLICKHOUSE_USER=straw
CLICKHOUSE_PASSWORD=example
STRAW_API_KEY_PEPPER=example
STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64=example
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=example
ENV

docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config -q

rendered="$(docker compose --env-file "$tmp_env" -f deploy/production/compose.yml config)"
printf '%s\n' "$rendered" | grep -q 'target: 8081'
printf '%s\n' "$rendered" | grep -q 'published: "8081"'
printf '%s\n' "$rendered" | grep -q 'target: 8082'
printf '%s\n' "$rendered" | grep -q 'published: "8082"'
if printf '%s\n' "$rendered" | grep -q 'target: 8083'; then
	echo "production template must not publish P2 MITM port 8083" >&2
	exit 1
fi

go test ./internal/config

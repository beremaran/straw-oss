#!/bin/bash
set -euo pipefail

host=${CLICKHOUSE_HOST:-127.0.0.1}
port=${CLICKHOUSE_PORT:-9000}
user=${CLICKHOUSE_USER:-default}
password=${CLICKHOUSE_PASSWORD:-}

client=(clickhouse-client --host "$host" --port "$port" --user "$user")
if [[ -n "$password" ]]; then
  client+=(--password "$password")
fi

for _ in {1..10}; do
  if output=$("${client[@]}" --multiquery 2>&1 <<'SQL'
ALTER TABLE straw.request_events
    ADD COLUMN IF NOT EXISTS requested_fingerprint_profile LowCardinality(String) DEFAULT '' AFTER selected_executor;

ALTER TABLE straw.request_events
    ADD COLUMN IF NOT EXISTS selected_fingerprint_profile LowCardinality(String) DEFAULT '' AFTER requested_fingerprint_profile;

ALTER TABLE straw.request_events
    ADD COLUMN IF NOT EXISTS executed_fingerprint_profile LowCardinality(String) DEFAULT '' AFTER selected_fingerprint_profile;
SQL
  ); then
    exit 0
  fi
  sleep 1
done

printf '%s\n' "$output" >&2
exit 1

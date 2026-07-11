#!/bin/bash
set -euo pipefail

INFRA_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
SCHEMA="$INFRA_DIR/clickhouse-schema.sql"
MIGRATION="$INFRA_DIR/scripts/migrate-clickhouse.sh"
IMAGE=${CLICKHOUSE_TEST_IMAGE:-clickhouse/clickhouse-server:24-alpine}
EXPECTED_COLUMNS=(
  requested_fingerprint_profile
  selected_fingerprint_profile
  executed_fingerprint_profile
)
CONTAINERS=()
VOLUMES=()

cleanup() {
  if ((${#CONTAINERS[@]})); then
    docker rm -f "${CONTAINERS[@]}" >/dev/null 2>&1 || true
  fi
  if ((${#VOLUMES[@]})); then
    docker volume rm "${VOLUMES[@]}" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

for column in "${EXPECTED_COLUMNS[@]}"; do
  grep -Eq "^[[:space:]]*$column[[:space:]]+LowCardinality\(String\)" "$SCHEMA" || {
    echo "canonical schema is missing $column" >&2
    exit 1
  }
done

[[ -x "$MIGRATION" ]] || {
  echo "missing executable migration: $MIGRATION" >&2
  exit 1
}

command -v docker >/dev/null 2>&1 || {
  echo "docker is required for ClickHouse migration checks" >&2
  exit 1
}
docker info >/dev/null 2>&1 || {
  echo "docker daemon is required for ClickHouse migration checks" >&2
  exit 1
}

wait_for_clickhouse() {
  local container=$1
  for _ in {1..60}; do
    if docker exec "$container" clickhouse-client --query 'SELECT 1' >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  echo "ClickHouse did not become ready: $container" >&2
  exit 1
}

wait_for_request_events() {
  local container=$1
  for _ in {1..60}; do
    if [[ $(docker exec "$container" clickhouse-client --query \
      "SELECT count() FROM system.tables WHERE database = 'straw' AND name = 'request_events'" 2>/dev/null) == 1 ]]; then
      return
    fi
    sleep 1
  done
  echo "straw.request_events did not become ready: $container" >&2
  exit 1
}

run_migration() {
  local container=$1
  local output
  for _ in {1..10}; do
    if output=$(docker exec "$container" bash /migrate-clickhouse.sh 2>&1); then
      return
    fi
    sleep 1
  done
  printf '%s\n' "$output" >&2
  echo "ClickHouse migration did not complete: $container" >&2
  exit 1
}

assert_columns() {
  local container=$1
  local count
  count=$(docker exec "$container" clickhouse-client --query \
    "SELECT count() FROM system.columns WHERE database = 'straw' AND table = 'request_events' AND name IN ('requested_fingerprint_profile', 'selected_fingerprint_profile', 'executed_fingerprint_profile') AND type = 'LowCardinality(String)' AND default_kind = 'DEFAULT' AND hex(default_expression) = '2727'")
  [[ "$count" == 3 ]] || {
    echo "profile evidence columns/defaults are incomplete in $container" >&2
    exit 1
  }
}

suffix="${RANDOM}-$$"
clean="straw-clickhouse-clean-$suffix"
CONTAINERS+=("$clean")
docker run -d --name "$clean" \
  -v "$SCHEMA:/docker-entrypoint-initdb.d/schema.sql:ro" \
  -v "$MIGRATION:/migrate-clickhouse.sh:ro" \
  "$IMAGE" >/dev/null
wait_for_clickhouse "$clean"
wait_for_request_events "$clean"
run_migration "$clean"
run_migration "$clean"
assert_columns "$clean"

volume="straw-clickhouse-existing-$suffix"
existing="straw-clickhouse-existing-$suffix"
VOLUMES+=("$volume")
CONTAINERS+=("$existing")
docker volume create "$volume" >/dev/null
docker run -d --name "$existing" -v "$volume:/var/lib/clickhouse" "$IMAGE" >/dev/null
wait_for_clickhouse "$existing"
sed '/fingerprint_profile/d' "$SCHEMA" | docker exec -i "$existing" clickhouse-client --multiquery
docker rm -f "$existing" >/dev/null

docker run -d --name "$existing" \
  -v "$volume:/var/lib/clickhouse" \
  -v "$SCHEMA:/docker-entrypoint-initdb.d/schema.sql:ro" \
  -v "$MIGRATION:/migrate-clickhouse.sh:ro" \
  "$IMAGE" >/dev/null
wait_for_clickhouse "$existing"
wait_for_request_events "$existing"
run_migration "$existing"
run_migration "$existing"
assert_columns "$existing"

echo "ClickHouse clean-schema and existing-volume migration checks passed."

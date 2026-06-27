#!/usr/bin/env bash

set -euo pipefail

# Find repository root directory (one level up from this script's location)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

COMPOSE_FILE="test/load/docker-compose.yml"

cleanup() {
  echo "Cleaning up containers and volumes..."
  docker compose -f "${COMPOSE_FILE}" down --volumes
}

# Always clean up on exit, even if there's a failure
trap cleanup EXIT

echo "Starting load test containers..."
docker compose -f "${COMPOSE_FILE}" up -d relay endpoint-1 endpoint-2 endpoint-3 endpoint-4 mock-target

echo "Waiting for relay to be healthy..."
until docker compose -f "${COMPOSE_FILE}" exec -T relay curl -sf http://localhost:8080/healthz >/dev/null 2>&1; do
  sleep 1
done

echo "Seeding API key..."
# Execute the seed request on the relay container, parse the raw key
RESPONSE=$(docker compose -f "${COMPOSE_FILE}" exec -T relay curl -s -X POST http://localhost:8081/admin/api-keys \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin-secret-token" \
  -d '{"name":"load-test","scopes":["*"],"rate_limit_override":10000}')

RAW_KEY=$(echo "${RESPONSE}" | grep -o '"raw_key":"[^"]*"' | cut -d'"' -f4 || true)

if [ -z "${RAW_KEY}" ]; then
  echo "Error: Failed to obtain API key from response: ${RESPONSE}" >&2
  exit 1
fi

echo "API key seeded successfully."

echo "Seeding routing rules..."
docker compose -f "${COMPOSE_FILE}" exec -T relay curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8081/admin/rules \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer admin-secret-token" \
  -d '{"name":"load-test-rule","required_tags":["type:datacenter","region:us"],"priority":100,"endpoint_pools":[{"tier":1,"endpoints":["load-test-1","load-test-2","load-test-3","load-test-4"],"max_retries":0}],"is_active":true}'

echo "Routing rules seeded successfully."

echo "Waiting for relay and endpoints to be fully ready..."
i=0
while true; do
  HTTP_CODE=$(docker compose -f "${COMPOSE_FILE}" exec -T relay curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/v1/request \
    -H "Authorization: Bearer ${RAW_KEY}" \
    -H "Content-Type: application/json" \
    -H "X-Relay-Tags: type:datacenter,region:us" \
    -d '{"url":"http://mock-target:80"}' || true)
  
  if [ "${HTTP_CODE}" -eq 200 ]; then
    break
  fi
  
  i=$((i + 1))
  if [ "$i" -ge 24 ]; then
    echo "Error: Relay/endpoints did not become ready in time (HTTP code ${HTTP_CODE})" >&2
    exit 1
  fi
  echo "Waiting for 200 OK... (attempt ${i}/24)"
  sleep 5
done

echo "Running k6 load test..."
docker compose -f "${COMPOSE_FILE}" run --rm \
  -e API_TOKEN="${RAW_KEY}" \
  -e BASE_URL=http://relay:8080 \
  k6 run /script.js

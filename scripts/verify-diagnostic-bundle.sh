#!/usr/bin/env bash
set -Eeuo pipefail

canary_token='diagnostic-token-canary'
canary_url='https://private.example.test/secret-path'
canary_header='X-Private-Header: canary'
canary_body='private-body-canary'
canary_backend='redis://user:password@private-host:6379/0'
output=$(STRAW_AUTH_TOKEN=$canary_token STRAW_TARGET_URL=$canary_url STRAW_REQUEST_HEADER=$canary_header \
  STRAW_REQUEST_BODY=$canary_body STRAW_REDIS_URL=$canary_backend PROFILE=receipts ./scripts/collect-diagnostics.sh)

for canary in "$canary_token" "$canary_url" "$canary_header" "$canary_body" "$canary_backend"; do
  if grep -Fq "$canary" <<<"$output"; then
    echo 'diagnostic bundle leaked a synthetic sensitive value' >&2
    exit 1
  fi
done

expected='collected_at profile straw_revision os_arch go_version docker_client compose_version control_health_status control_readiness_status'
actual=$(printf '%s\n' "$output" | cut -d= -f1 | tr '\n' ' ' | sed 's/ $//')
[[ $actual == "$expected" ]]
echo 'diagnostic bundle allowlist and synthetic-secret exclusion: passed'

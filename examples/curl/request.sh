#!/usr/bin/env bash
set -euo pipefail

base_url=${STRAW_BASE_URL:-http://localhost:8080}
# Any endpoint that echoes the request works here. httpbin.org has been serving
# 503 since mid-2026, so the default is httpbingo.org, the same project's
# maintained instance. Override it to echo somewhere else.
target_url=${STRAW_EXAMPLE_TARGET_URL:-https://httpbingo.org/anything}
curl_args=(--fail --silent --show-error --max-time 30)
if [[ -n ${STRAW_AUTH_TOKEN:-} ]]; then curl_args+=(-H "Authorization: Bearer ${STRAW_AUTH_TOKEN}"); fi
curl "${curl_args[@]}" \
  -H 'Content-Type: application/json' \
  --data "{\"method\":\"POST\",\"url\":\"${target_url}\",\"headers\":[{\"name\":\"X-Example-Trace\",\"value_base64\":\"b25l\"},{\"name\":\"X-Example-Trace\",\"value_base64\":\"dHdv\"}],\"body\":{\"mode\":\"inline_base64\",\"data_base64\":\"aGVsbG8=\"},\"timeout_ms\":15000}" \
  "${base_url}/api/v1/requests"

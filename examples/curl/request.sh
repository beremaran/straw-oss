#!/usr/bin/env bash
set -euo pipefail

base_url=${STRAW_BASE_URL:-http://localhost:8080}
curl_args=(--fail --silent --show-error --max-time 30)
if [[ -n ${STRAW_AUTH_TOKEN:-} ]]; then curl_args+=(-H "Authorization: Bearer ${STRAW_AUTH_TOKEN}"); fi
curl "${curl_args[@]}" \
  -H 'Content-Type: application/json' \
  --data '{"method":"POST","url":"https://httpbin.org/anything","headers":[{"name":"X-Example-Trace","value_base64":"b25l"},{"name":"X-Example-Trace","value_base64":"dHdv"}],"body":{"mode":"inline_base64","data_base64":"aGVsbG8="},"timeout_ms":15000}' \
  "${base_url}/api/v1/requests"

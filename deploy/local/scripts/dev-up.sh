#!/usr/bin/env bash

set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/dev-common.sh"

check_prerequisites
printf 'local stack: building and starting Straw + NATS...\n'
compose up -d --build >/dev/null || die "Compose failed; run 'make dev-status' for details"
wait_for_url "http://127.0.0.1:${CONTROL_METRICS_PORT}/readyz" "Control"
printf 'local stack: ready\n'
printf 'try: curl -sS -H "Content-Type: application/json" -d '\''{"method":"GET","url":"https://example.com"}'\'' http://localhost:%s/api/v1/requests\n' "$CONTROL_API_PORT"

#!/usr/bin/env bash

set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/dev-common.sh"

check_prerequisites
compose ps
printf '\nControl readiness: '
if curl --fail --silent --max-time 3 "http://127.0.0.1:${CONTROL_METRICS_PORT}/readyz" >/dev/null; then
  printf 'ready\n'
else
  printf 'not ready\n'
  exit 1
fi

#!/usr/bin/env bash

set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/dev-common.sh"

check_prerequisites
compose down -v
printf 'local stack: removed containers and volumes\n'

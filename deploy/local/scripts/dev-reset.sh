#!/usr/bin/env bash

set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "$SCRIPT_DIR/dev-common.sh"
check_prerequisites
if [[ -f "$LOCAL_ENV" ]]; then
  chmod 600 "$LOCAL_ENV"
  compose down -v >/dev/null
else
  args=(-f "$COMPOSE_BASE")
  if [[ -n "$COMPOSE_LOCAL" ]]; then args+=(-f "$COMPOSE_LOCAL"); fi
  docker compose "${args[@]}" down -v >/dev/null
fi
rm -f "$LOCAL_ENV"
printf 'local stack: removed containers, volumes, and generated credentials\n'

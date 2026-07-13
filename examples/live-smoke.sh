#!/usr/bin/env bash
set -Eeuo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
project=straw-examples-smoke
compose=(docker compose -p "$project" -f deploy/local/docker-compose.yml)
up_args=(-d --wait)
if [[ ${STRAW_SMOKE_BUILD:-true} == true ]]; then
  up_args+=(--build)
else
  up_args+=(--no-build)
fi
cleanup() { "${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT

"${compose[@]}" up "${up_args[@]}"
examples/curl/request.sh | grep -q '"status":200'
examples/cli/request.sh | grep -q '"status":200'
go run ./examples/go | grep -q '^200 '
uv run --frozen python examples/python/request.py | grep -q '^200 '

echo 'live curl, CLI, Go, and Python examples: passed'

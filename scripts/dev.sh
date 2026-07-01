#!/usr/bin/env sh
set -eu

DC="docker compose -f docker/docker-compose.dev.yml"

case "${1:-}" in
  up)
    $DC up -d
    ;;
  down)
    $DC down
    ;;
  shell)
    $DC run --rm dev sh
    ;;
  build)
    make build
    ;;
  test)
    make test
    ;;
  lint)
    make lint
    ;;
  *)
    echo "usage: $0 {up|down|shell|build|test|lint}" >&2
    exit 2
    ;;
esac

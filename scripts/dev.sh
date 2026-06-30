#!/usr/bin/env bash
set -euo pipefail

DC="docker compose -f docker/docker-compose.dev.yml"

usage() {
  cat <<EOF
Usage: $(basename "$0") <command> [args...]

Commands:
  up          Start dev environment (infra + dev shell)
  down        Stop dev environment
  shell       Open an interactive dev shell
  build       Build relay and endpoint binaries
  test        Run tests with race detector
  lint        Run golangci-lint
  docs        Build documentation
  serve-docs  Serve documentation locally
  db-migrate  Run database migrations (relay)
  db-reset    Drop and recreate the database
  logs        View container logs
  ps          List running containers
  clean       Remove volumes and build cache
  help        Show this help message

Examples:
  $(basename "$0") up          # Start infra and open dev shell
  $(basename "$0") test        # Run tests in dev container
  $(basename "$0") lint        # Run linter in dev container
  $(basename "$0") shell       # Open interactive shell
  $(basename "$0") logs -f     # Follow logs
EOF
}

run_in_dev() {
  $DC run --rm dev "$@"
}

case "${1:-help}" in
  up)
    echo "Starting dev environment..."
    $DC up -d postgres nats redis
    echo "Infra started. Opening dev shell..."
    $DC run --rm dev sh
    ;;
  down)
    $DC down
    echo "Dev environment stopped."
    ;;
  shell)
    $DC run --rm dev sh
    ;;
  build)
    run_in_dev make dev-build
    ;;
  test)
    run_in_dev make dev-test
    ;;
  lint)
    run_in_dev make dev-lint
    ;;
  docs)
    run_in_dev make dev-docs
    ;;
  serve-docs)
    $DC run --rm -p 8000:8000 dev uvx --with mkdocs-material mkdocs serve --dev-addr=0.0.0.0:8000
    ;;
  db-migrate)
    run_in_dev go run ./cmd/relay migrate up
    ;;
  db-reset)
    echo "Dropping and recreating database..."
    docker exec -it "$($DC ps -q postgres 2>/dev/null | head -1)" psql -U straw -d straw -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;" 2>/dev/null || true
    echo "Database reset complete."
    ;;
  logs)
    shift
    $DC logs "$@"
    ;;
  ps)
    $DC ps
    ;;
  clean)
    echo "Removing volumes..."
    $DC down -v
    echo "Done."
    ;;
  help|--help|-h)
    usage
    ;;
  *)
    echo "Unknown command: $1"
    usage
    exit 1
    ;;
esac

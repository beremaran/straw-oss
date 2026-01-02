#!/usr/bin/env bash
# Migration helper script for Straw Proxy Server
# Usage: ./scripts/migrate.sh [up|down|status|create|redo|reset]

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
MIGRATIONS_DIR="${PROJECT_ROOT}/internal/infra/postgres/migrations"

# Load environment variables if .env file exists
if [[ -f "${PROJECT_ROOT}/.env" ]]; then
    set -a
    source "${PROJECT_ROOT}/.env"
    set +a
elif [[ -f "${PROJECT_ROOT}/.env.local" ]]; then
    set -a
    source "${PROJECT_ROOT}/.env.local"
    set +a
fi

# Default database URL (can be overridden by environment)
POSTGRES_DSN="${POSTGRES_DSN:-postgres://postgres:postgres@localhost:5432/straw_proxy?sslmode=disable}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if goose is installed
check_goose() {
    if ! command -v goose &> /dev/null; then
        log_warn "goose not found in PATH, attempting to use go run..."
        GOOSE_CMD="go run github.com/pressly/goose/v3/cmd/goose@latest"
    else
        GOOSE_CMD="goose"
    fi
}

# Run goose with standard options
run_goose() {
    ${GOOSE_CMD} -dir "${MIGRATIONS_DIR}" postgres "${POSTGRES_DSN}" "$@"
}

# Show usage
usage() {
    echo "Usage: $0 [command] [options]"
    echo ""
    echo "Commands:"
    echo "  up              Apply all pending migrations"
    echo "  down            Roll back the last migration"
    echo "  down-all        Roll back all migrations"
    echo "  status          Show migration status"
    echo "  create NAME     Create a new migration file"
    echo "  redo            Roll back and re-apply the last migration"
    echo "  reset           Roll back all migrations and re-apply"
    echo "  version         Show current migration version"
    echo ""
    echo "Environment Variables:"
    echo "  POSTGRES_DSN    Database connection string"
    echo ""
    echo "Examples:"
    echo "  $0 up"
    echo "  $0 create add_user_roles"
    echo "  POSTGRES_DSN='postgres://user:pass@host/db' $0 status"
}

# Main
main() {
    check_goose

    if [[ $# -lt 1 ]]; then
        usage
        exit 1
    fi

    local command="$1"
    shift

    case "$command" in
        up)
            log_info "Applying pending migrations..."
            run_goose up
            log_info "Migrations applied successfully!"
            ;;
        down)
            log_info "Rolling back last migration..."
            run_goose down
            log_info "Migration rolled back successfully!"
            ;;
        down-all)
            log_warn "Rolling back ALL migrations..."
            run_goose down-to 0
            log_info "All migrations rolled back!"
            ;;
        status)
            log_info "Migration status:"
            run_goose status
            ;;
        create)
            if [[ $# -lt 1 ]]; then
                log_error "Migration name required"
                echo "Usage: $0 create NAME"
                exit 1
            fi
            local name="$1"
            log_info "Creating new migration: $name"
            run_goose create "$name" sql
            log_info "Migration created!"
            ;;
        redo)
            log_info "Re-applying last migration..."
            run_goose redo
            log_info "Migration re-applied successfully!"
            ;;
        reset)
            log_warn "Resetting all migrations (down-to 0, then up)..."
            run_goose down-to 0
            run_goose up
            log_info "Migrations reset successfully!"
            ;;
        version)
            run_goose version
            ;;
        -h|--help|help)
            usage
            ;;
        *)
            log_error "Unknown command: $command"
            usage
            exit 1
            ;;
    esac
}

main "$@"

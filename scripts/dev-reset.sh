#!/bin/bash
# Reset the Straw Proxy development stack (removes all data)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "⚠️  This will delete all development data (PostgreSQL, Redis, RabbitMQ)"
echo ""

# Check if --force flag is passed
if [[ "$1" != "--force" && "$1" != "-f" ]]; then
  read -p "Are you sure? (y/N) " -n 1 -r
  echo
  if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
  fi
fi

echo "🛑 Stopping containers and removing volumes..."
docker compose down -v

echo "🚀 Starting fresh development stack..."
"$SCRIPT_DIR/dev-up.sh"

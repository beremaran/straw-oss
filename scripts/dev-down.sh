#!/bin/bash
# Stop the Straw Proxy development stack

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "🛑 Stopping Straw development stack..."
docker compose down

echo "✅ Development stack stopped"

#!/bin/bash
# Start the Straw Proxy development stack

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$PROJECT_DIR"

echo "🚀 Starting Straw development stack..."
docker compose up -d

echo "⏳ Waiting for services to be healthy..."

# Wait for postgres
until docker compose exec -T postgres pg_isready -U straw -d straw > /dev/null 2>&1; do
  echo "  Waiting for PostgreSQL..."
  sleep 2
done
echo "  ✅ PostgreSQL is ready"

# Wait for redis
until docker compose exec -T redis redis-cli ping > /dev/null 2>&1; do
  echo "  Waiting for Redis..."
  sleep 2
done
echo "  ✅ Redis is ready"

# Wait for rabbitmq
until docker compose exec -T rabbitmq rabbitmq-diagnostics -q ping > /dev/null 2>&1; do
  echo "  Waiting for RabbitMQ..."
  sleep 2
done
echo "  ✅ RabbitMQ is ready"

echo ""
echo "🎉 All services are running and healthy!"
echo ""
echo "Services:"
echo "  PostgreSQL: localhost:5432 (user: straw, password: straw, db: straw)"
echo "  Redis:      localhost:6379"
echo "  RabbitMQ:   localhost:5672 (management: http://localhost:15672)"

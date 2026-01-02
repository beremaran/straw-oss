# Straw Proxy Server Makefile

.PHONY: help dev-up dev-down dev-reset dev-logs dev-ps build test lint docker-build docker-build-server docker-build-endpoint

# Default target
help:
	@echo "Straw Proxy Server - Development Commands"
	@echo ""
	@echo "Development Stack:"
	@echo "  make dev-up      Start development stack (PostgreSQL, Redis, RabbitMQ)"
	@echo "  make dev-down    Stop development stack"
	@echo "  make dev-reset   Reset all data and restart"
	@echo "  make dev-logs    Tail logs from all services"
	@echo "  make dev-ps      Show service status"
	@echo ""
	@echo "Build & Test:"
	@echo "  make build       Build all binaries"
	@echo "  make test        Run all tests"
	@echo "  make lint        Run linters"
	@echo "  make docker-build         Build all docker images"
	@echo "  make docker-build-server  Build server docker image"
	@echo "  make docker-build-endpoint Build endpoint docker image"
	@echo ""
	@echo "Database Migrations:"
	@echo "  make migrate-up          Apply pending migrations"
	@echo "  make migrate-down        Rollback last migration"
	@echo "  make migrate-reset       Reset database (rollback all + up)"
	@echo "  make migrate-status      Show migration status"
	@echo "  make migrate-create name=N Create new migration file"
	@echo ""

# Development stack targets
dev-up:
	@./scripts/dev-up.sh

dev-down:
	@./scripts/dev-down.sh

dev-reset:
	@./scripts/dev-reset.sh --force

dev-logs:
	docker compose logs -f

dev-ps:
	docker compose ps

docker-build: docker-build-server docker-build-endpoint

docker-build-server:
	@./scripts/build-docker.sh server

docker-build-endpoint:
	@./scripts/build-docker.sh endpoint

# Build targets
build: build-gui
	@echo "Building server..."
	go build -o bin/relay-server ./cmd/relay-server
	@echo "Building endpoint..."
	go build -o bin/endpoint ./cmd/endpoint

build-gui:
	@echo "Building endpoint-gui..."
	go build -o bin/endpoint-gui ./cmd/endpoint-gui

test:
	go test -race -v ./...

lint:
	golangci-lint run ./...
migrate-up:
	@./scripts/migrate.sh up

migrate-down:
	@./scripts/migrate.sh down

migrate-reset:
	@./scripts/migrate.sh reset

migrate-status:
	@./scripts/migrate.sh status

migrate-create:
	@if [ -z "$(name)" ]; then echo "Error: name argument required. Usage: make migrate-create name=my_migration"; exit 1; fi
	@./scripts/migrate.sh create $(name)



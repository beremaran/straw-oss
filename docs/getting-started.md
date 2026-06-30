# Getting Started

This guide walks you through setting up Straw Proxy locally, running it with Docker Compose, building from source, and executing the test suites.

---

## Prerequisites

To run Straw Proxy locally, you need the following dependencies:

* **Go**: Version `1.25` or later.
* **PostgreSQL**: Version `15` or later (stores API keys, routing rules, usage audits).
* **Redis**: Version `7.x` or later (stores sessions, cache, rate limit quotas, endpoint health status).
* **NATS Server**: Version `2.10` or later (serves as the message broker).
* **Docker & Docker Compose**: (Optional, for containerized local setups).

---

## 🐋 1. Spin Up with Docker Compose

The easiest way to get a full environment running (Postgres, Redis, NATS, Relay Server, and Endpoint Worker) is via Docker Compose.

Create a `docker-compose.yml` in the root of the project:

```yaml
version: '3.8'

services:
  # Message Broker
  nats:
    image: nats:2.10-alpine
    container_name: straw-nats
    ports:
      - "4222:4222"
      - "8222:8222" # Monitor port
    command: "--js" # Enable JetStream (if required)

  # Cache & Session Store
  redis:
    image: redis:7-alpine
    container_name: straw-redis
    ports:
      - "6379:6379"

  # Relational Database
  postgres:
    image: postgres:16-alpine
    container_name: straw-postgres
    environment:
      POSTGRES_DB: straw_proxy
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d straw_proxy"]
      interval: 5s
      timeout: 5s
      retries: 5

  # Control/Relay Gateway
  relay-server:
    build:
      context: .
      dockerfile: docker/Dockerfile
      args:
        BINARY_NAME: relay
    container_name: straw-relay
    ports:
      - "8080:8080" # Client API
      - "8081:8081" # Management API
      - "9090:9090" # Metrics Port
    environment:
      - POSTGRES_DSN=postgres://postgres:postgres@postgres:5432/straw_proxy?sslmode=disable
      - REDIS_ADDR=redis:6379
      - NATS_URL=nats://nats:4222
      - HMAC_SECRET=your-secure-hmac-signing-secret
      - DB_AUTO_MIGRATE=true
      - ALLOW_PRIVATE_IPS=true
      - METRICS_ENABLED=true
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started
      nats:
        condition: service_started

  # Egress Worker Node
  endpoint-worker:
    build:
      context: .
      dockerfile: docker/Dockerfile
      args:
        BINARY_NAME: endpoint

---

## 🐳 Fully Dockerized Development Environment

Straw Proxy uses a fully dockerized development workflow. **No local dependencies are required** — all tooling (Go, Node.js, Python, linters, formatters) runs inside a single dev container. This ensures every contributor works in an identical environment.

### Quick Start

Start the development environment:

```bash
./scripts/dev.sh up
```

This starts PostgreSQL, Redis, NATS, and an interactive dev shell with all tools pre-installed.

### Makefile Targets

All development targets are available as `make dev-*` commands:

```bash
make dev-build      # Build relay and endpoint binaries
make dev-test       # Run tests with race detector
make dev-lint       # Run golangci-lint
make dev-docs       # Build documentation
make dev-shell      # Open interactive dev shell
```

When running **on the host**, these targets use Docker Compose to execute inside the dev container. When running **inside the dev container**, they execute natively.

### Interactive Shell

Open a dev shell with all tools available:

```bash
./scripts/dev.sh shell
# or
make dev-shell
```

Inside the shell, all standard commands work directly:

```bash
make dev-build
make dev-test
make dev-lint
go test -race ./...
golangci-lint run ./...
```

### Infrastructure Services

The dev environment includes managed infrastructure services:

| Service    | Port  | Purpose                          |
|------------|-------|----------------------------------|
| PostgreSQL | 5432  | API keys, routing rules, audits  |
| Redis      | 6379  | Sessions, cache, rate limits     |
| NATS       | 4222  | Message broker                   |

Start/stop infrastructure independently:

```bash
./scripts/dev.sh up    # Start infra only
./scripts/dev.sh down  # Stop infra
```

### Docker Compose Configuration

The development environment is defined in `docker/docker-compose.dev.yml`:

- **`dev`** — Full dev container with Go, Node.js, Python, linters, and all tooling
- **`postgres`** — PostgreSQL 16 with health checks
- **`nats`** — NATS 2.10 with JetStream enabled
- **`redis`** — Redis 7

Volumes are configured for fast builds (Go module cache, build cache, npm cache) and persistent data (PostgreSQL, NATS, Redis).

### Pre-commit Hooks

Pre-commit hooks run inside the dev container by default:

```bash
pre-commit install
pre-commit run --all-files
```

Hooks: `make dev-build`, `make dev-test`, `make dev-lint`, and frontend lint — all execute inside the dockerized environment.
      - HMAC_SECRET=your-secure-hmac-signing-secret
      - ENDPOINT_TAGS=type:residential,region:us
      - CONCURRENCY_LIMIT=25
      - METRICS_ENABLED=true
      - METRICS_PORT=9091
    depends_on:
      - relay-server
```

Start the stack:

```bash
docker compose up -d
```

---

## 🛠️ 2. Running Locally from Source

If you prefer to run Straw Proxy processes directly on your host machine, follow these steps:

### Build Binaries

Use the provided `Makefile` to compile both target services:

```bash
make build
```

This compiles:
* `bin/relay` - The Relay Server (Orchestrator).
* `bin/endpoint` - The Endpoint Worker.

### Setup Databases

1. **Start dependencies**: Make sure your local Postgres, Redis, and NATS servers are running:
   ```bash
   # Example using Docker
   docker run -d --name local-postgres -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=straw_proxy postgres:16-alpine
   docker run -d --name local-redis -p 6379:6379 redis:7-alpine
   docker run -d --name local-nats -p 4222:4222 nats:2.10-alpine --js
   ```

2. **Configure environment variables**: Copy the example configuration files and fill in values:
   ```bash
   cp config/.relay.env.example .relay.env
   cp config/.endpoint.env.example .endpoint.env
   ```

3. **Run database migrations**: Set `DB_AUTO_MIGRATE=true` on the Relay Server startup to automatically apply all embedded SQL schema migrations on startup, or use a goose-compatible migration tool.

### Start the Services

In terminal 1, launch the Relay Server:
```bash
set -a; source .relay.env; set +a
export POSTGRES_DSN="postgres://postgres:postgres@localhost:5432/straw_proxy?sslmode=disable"
export DB_AUTO_MIGRATE=true
./bin/relay
```

In terminal 2, launch the Endpoint Worker:
```bash
set -a; source .endpoint.env; set +a
export ENDPOINT_ID="worker-us-residential-01"
export ENDPOINT_TAGS="type:residential,region:us"
./bin/endpoint
```

---

## 🧪 3. Running Tests

Straw Proxy has a comprehensive testing system.

### Unit Tests

Run all unit tests in the codebase:

```bash
make test
```

### Integration Tests

The integration test suite uses [Testcontainers for Go](https://golang.testcontainers.org/) to spin up isolated container instances of PostgreSQL, Redis, and NATS. This guarantees exact behavior verification without polluting your local system state.

Make sure Docker is running on your system, then execute:

```bash
go test -v ./test/integration/...
```

### Code Linter & Formatting

Ensure your code contributions adhere to the style guide:

```bash
make format
make lint
```

---

## 🔌 4. API Clients & Code Generation

Straw Proxy publishes a comprehensive **OpenAPI 3.0.3 specification** located at `api/openapi.yaml`. This specification can be used to generate clean API client SDKs for any programming language (Go, TypeScript, Python, Java, etc.) to communicate with the Client and Management APIs.

### Validate the OpenAPI Specification
To validate the OpenAPI schema format, run:
```bash
make docs
```
This runs the `@redocly/cli` linter on the spec file.

### Specification Drift Tests
To ensure the OpenAPI specification never drifts out of sync with the actual Go DTO structs used by the server runtime, we have a contract test suite:
* File: `test/contract/openapi_drift_test.go`

This test suite loads `api/openapi.yaml`, normalizes its schema components, and validates actual JSON-marshaled Go DTO instances against the corresponding schema definitions using `gojsonschema`.

Any changes to HTTP request/response payloads in Go code (or in the OpenAPI specification) will automatically trigger validation failures in standard tests unless both are kept in sync.

Run the spec drift tests as part of the normal test target:
```bash
make test
```

### Generate Client SDKs
We have bundled ready-to-run client generation tasks in the `Makefile`. Run:
```bash
make generate-clients
```

This uses the `@openapitools/openapi-generator-cli` tool (via Node/npx) to automatically generate:
* **TypeScript Client** (`client/typescript/`)
* **Go Client** (`client/go/`)

### Generating for Other Languages
You can also run the generator manually to create SDKs for other target languages (e.g. Python):
```bash
npx @openapitools/openapi-generator-cli generate \
  -i api/openapi.yaml \
  -g python \
  -o client/python
```

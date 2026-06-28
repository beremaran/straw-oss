# 🥤 Straw Proxy

[![Go Version](https://img.shields.io/github/go-mod/go-version/beremaran/straw?color=00ADD8&logo=go)](https://golang.org)
[![Docker Image](https://img.shields.io/badge/docker-publish-blue?logo=docker&logoColor=white)](https://github.com/beremaran/straw/pkgs/container/straw)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Docs](https://img.shields.io/badge/docs-mkdocs-green.svg)](docs/index.md)

**Straw Proxy** is a distributed, high-performance HTTP proxy mesh and routing orchestrator designed for web scraping, automated testing, and anti-bot bypass. By decoupling the control/routing layer from the physical execution/egress nodes, Straw Proxy allows you to scale, orchestrate, and secure distributed scraper traffic seamlessly.

---

## 🎨 System Architecture

Unlike traditional proxy servers that route traffic through a single server, Straw Proxy splits execution into a central **Control Plane (Relay)** and a distributed **Execution Plane (Endpoints)**:

```mermaid
graph TD
    Client[HTTP Client] -->|1. HTTP Request| Relay[Relay Server]
    subgraph Control Layer (Relay Server)
        Relay --> Auth[API Key Auth & Limits]
        Relay --> Router[Routing & Priority Engine]
        Relay --> Filter[ABP Tracker Filter]
    end
    Relay -->|2. Task Publish| Broker((NATS Broker))
    subgraph Execution Layer (Distributed Workers)
        Broker -->|3. Job Dispatch| Worker1[Worker A - Residential US]
        Broker -->|3. Job Dispatch| Worker2[Worker B - Datacenter EU]
        Worker1 -->|4. Spoofed Request| Web1[Target Website]
        Worker2 -->|4. Spoofed Request| Web2[Target Website]
    end
```

1. **Relay Server (Orchestrator)**: The entry gateway. It handles API authentication, rate limiting, AdBlock filtering, session pinning, and task scheduling.
2. **NATS Message Broker**: Dispatches tasks to workers and streams responses back in real time.
3. **Endpoint Workers**: Lightweight, stateless worker nodes that subscribe to task streams, execute HTTPS requests with custom browser fingerprints (TLS/JA3/JA4), and return the results.

---

## ✨ Key Features

* 🚀 **Decoupled Egress Scaling**: Run multiple lightweight worker nodes across residential, cellular, and datacenter networks without changes to your central API client.
* 🕵️ **Advanced Client Spoofing**: Built-in emulation for JA3/JA4 TLS fingerprints, HTTP/2 connection flows, and browser header presets to bypass Cloudflare, Akamai, and other anti-bot services.
* 🛑 **ABP Ad & Tracker Filtering**: Embedded AdBlock Plus engine filters ads, trackers, and telemetry directly at the Relay level, saving valuable worker egress bandwidth.
* 🗺️ **Smart Egress Routing**: Route proxy requests dynamically using tags (e.g. `type:residential`, `region:us`).
* 🔒 **Session Pinning (Sticky Sessions)**: Keep multi-request browser sessions pinned to the exact same egress worker node.
* 🔌 **Custom Worker SDK**: Implement your own custom egress handlers, proxies, and logging using our exported Go SDK packages.
* 📊 **Production-Ready Observability**: Built-in Prometheus metrics, OpenTelemetry traces, pprof profiling, and structured logging.
* 🛡️ **Resilience**: Integrated Redis-backed rate limiting, database circuit breakers, and automatic Server-Side Request Forgery (SSRF) protections.

---

## 🚀 Quick Start (Under 2 Minutes)

The fastest way to spin up the entire Straw Proxy stack is using **Docker Compose**.

### 1. Run the Stack

Start Postgres, Redis, NATS, a Relay Server, and a residential worker:

```bash
docker compose up -d
```

* *Default Client API Port*: `8080`
* *Default Admin API Port*: `8081`

### 2. Generate a Client API Key

Create an API key for your crawler clients using the Admin API:

```bash
curl -X POST http://localhost:8081/admin/api-keys \
  -H "Authorization: Bearer default-token-123456" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Scraper Key",
    "scopes": ["target:*", "type:residential"],
    "rate_limit_override": 100
  }'
```

Save the `raw_key` returned in the JSON response (e.g., `YOUR_CLIENT_API_KEY`).

### 3. Send a Proxied Request

Send requests through the proxy mesh via the Relay HTTP gateway:

```bash
curl -X POST http://localhost:8080/v1/request \
  -H "Authorization: Bearer YOUR_CLIENT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "method": "GET",
    "url": "https://httpbin.org/headers"
  }'
```

---

## 🔌 Building Custom Workers (Go SDK)

Straw Proxy exports its message protocol and worker interfaces as a modular Go SDK. You can compile your own custom workers integrating commercial residential proxy pools, rotators, or custom TLS spoofing:

```go
package main

import (
	"context"
	"log"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/pkg/endpoint"
)

func main() {
	// 1. Load configuration from environment
	cfg, err := config.LoadEndpointConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	// 2. Initialize a worker with custom request execution
	worker := endpoint.NewWorker(cfg, endpoint.WithRequestExecutor(myCustomExecutor))

	// 3. Start receiving and executing requests
	if err := worker.Start(context.Background()); err != nil {
		log.Fatalf("worker failed: %v", err)
	}
}
```

Refer to the [Custom Endpoint Developer Guide](docs/custom-endpoint.md) for full implementation details.

---

## 📁 Repository Structure

```text
├── api/                  # OpenAPI 3.0 specification schemas
├── cmd/
│   ├── relay-server/     # Control Plane entry point
│   └── endpoint/         # Data/Execution Plane default worker daemon
├── docs/                 # Extensive MkDocs markdown documentation
├── internal/             # Private application logic (routing, filters, DB)
├── pkg/                  # Public SDKs (Endpoint, NATS Broker, Protocol models)
├── scripts/              # Setup, linting, and load testing scripts
└── test/                 # Unit, contract, and integration test suites
```

---

## ⚙️ Configuration

Straw Proxy is configured using environment variables. Below is a summary of the core variables:

| Variable | Type | Description |
|---|---|---|
| `POSTGRES_DSN` | string | PostgreSQL database connection string (DSN). |
| `NATS_URL` | string | NATS message broker URL (e.g., `nats://localhost:4222`). |
| `HMAC_SECRET` | string | Shared signing key used for securing NATS task payloads. **Must be identical on Relay and Workers.** |
| `ENDPOINT_ID` | string | (Worker only) Unique identifier for the worker node. |
| `ENDPOINT_TAGS` | string | (Worker only) Comma-separated tags (e.g., `type:residential,region:us`). |

For a complete list of configuration options, check the [Configuration Reference](docs/configuration.md).

---

## 📚 Documentation

The `/docs` folder contains a comprehensive documentation suite. You can run the docs site locally using:

```bash
make docs-serve
```

* **[Getting Started](docs/getting-started.md)**: Extended setup and development environment guides.
* **[Architecture Overview](docs/architecture.md)**: In-depth sequence flows, schemas, and resilience patterns.
* **[Admin API Reference](docs/admin-api.md)**: Detailed API specs for key rotation, routing rules, and metrics.
* **[Custom Endpoint SDK](docs/custom-endpoint.md)**: Complete guide on building proprietary workers.

---

## 🤝 Contributing

We welcome contributions to Straw Proxy! Please read [CONTRIBUTING.md](CONTRIBUTING.md) to get started with our development workflow and code style guidelines.

---

## 📄 License

Straw Proxy is open-sourced software licensed under the [MIT License](LICENSE).


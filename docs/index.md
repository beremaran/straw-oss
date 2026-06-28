# Straw Proxy

Welcome to the **Straw Proxy** documentation site! 

Straw Proxy is a distributed, high-performance HTTP proxy mesh and orchestration system designed for scraping, web testing, and proxy routing with built-in client fingerprint spoofing and centralized administration.

---

## What is Straw Proxy?

Unlike traditional proxy servers that routing requests through a single server or simple proxy chain, Straw Proxy separates the **control/relay layer** from the **execution layer**:

```mermaid
graph TD
    Client[HTTP Client] -->|HTTP Request| Relay[Relay Server]
    subgraph Control Layer (Relay)
        Relay --> Auth[API Key Auth]
        Relay --> Router[Routing Engine]
        Relay --> Limiter[Rate Limiter]
        Relay --> Filter[ABP Ad Filter]
    end
    Relay -->|NATS Task Stream| Broker((NATS Broker))
    subgraph Execution Layer (Endpoints)
        Broker -->|Job Dispatch| Worker1[Worker A - Residential US]
        Broker -->|Job Dispatch| Worker2[Worker B - Datacenter EU]
        Worker1 -->|Spoofed TLS & Headers| Web1[Target Website]
        Worker2 -->|Spoofed TLS & Headers| Web2[Target Website]
    end
```

1. **Relay Server (Orchestrator)**: Acts as the entry gate for all HTTP client requests. It validates API keys, checks rate limits, inspects/cleans requests using AdBlock Plus filters, and routes them to appropriate workers based on tag/geography requirements or active sessions.
2. **NATS Broker**: Serves as the real-time message stream dispatcher between the Relay and Workers.
3. **Endpoint Workers**: Distributed worker processes that subscribe to task streams, execute actual target requests using specialized TLS/HTTP clients (supporting JA3/JA4 fingerprinting and rotating egress proxies), and return results back to the Relay.

---

## Key Features

* 🚀 **Decoupled Architecture**: Easily scale execution nodes (workers) without modifying the main API gateway.
* 🔒 **API Authentication & Rate Limiting**: Centralized API key manager with Redis-backed rate limiting.
* 🗺️ **Smart Routing & Session Pinning**: Route requests based on endpoint tags (e.g., `region:us`, `type:residential`) and pin multi-request browser sessions to the same worker node.
* 🕵️ **Advanced Client Spoofing**: Built-in JA3/JA4 fingerprint emulation and client header presets to bypass anti-bot and cloud protection systems.
* 🛑 **ABP Ad Filtering**: Embedded AdBlock Plus engines block trackers, ads, and telemetry scripts directly at the relay level to save egress bandwidth.
* 🔌 **Flexible SDK**: Build your own custom endpoint workers using our exported Go SDK packages.
* 📦 **Self-Updating Workers**: Automatically updates worker binaries via simple HTTP manifest checks.
* 📊 **Observability**: Exposes Prometheus metrics, pprof endpoints, structured slog logs, and OpenTelemetry trace exports.

---

## Where to Go Next?

* **[Getting Started](getting-started.md)**: Spin up Straw Proxy locally using Docker Compose or build from source.
* **[Architecture Overview](architecture.md)**: Explore the detailed sequence flow, database design, and message broker structure.
* **[Configuration Reference](configuration.md)**: Comprehensive details about env variables for the Relay and Worker.
* **[Admin API Reference](admin-api.md)**: Management of API keys, routing rules, usage metrics, and worker health.
* **[Custom Endpoint SDK](custom-endpoint.md)**: Step-by-step developer guide for compiling custom endpoint workers.

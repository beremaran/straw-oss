# Straw Proxy Server 🥤

> The distributed, passive-consumer proxy system for high-scale web scraping.

[![Go Report Card](https://goreportcard.com/badge/github.com/kwilabs/straw-proxy-server)](https://goreportcard.com/report/github.com/kwilabs/straw-proxy-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/kwilabs/straw-proxy-server)](https://go.dev/)

**Straw Proxy Server (V2)** is designed to solve the hardest problems in web scraping infrastructure: **concurrency, latency, and blocking**. Unlike traditional proxy chains, Straw uses a "passive consumer" model where endpoints connect *outbound* to the server, bypassing complex NAT and firewall configurations.

---

## 🚀 Why Straw?

Traditional proxies require port forwarding and struggle with stability. Straw flips the model:

* **⚡ Zero-Config Deployment**: Endpoints work behind any NAT, firewall, or 4G modem without port forwarding.
* **🕵️‍♂️ Advanced Fingerprinting**: Built-in TLS spoofing (JA3/JA4) to mimic real browsers (Chrome, Firefox, Safari) and bypass anti-bot protections.
* **🚅 High Performance**: Written in Go to handle thousands of concurrent connections with minimal latency.
* **🧠 Intelligent Routing**: Tag-based routing ensures your requests hit the right endpoint every time (e.g., `target:amazon`, `region:us`).

## ✨ Key Features

| Feature | Description |
| :--- | :--- |
| **Passive Connectivity** | Endpoints dial `out` to the server; no incoming ports needed. |
| **TLS Fingerprinting** | powered by `utls` to emulate legitimate browser handshakes. |
| **Smart Caching** | Redis-backed caching for high-speed config and session lookups. |
| **Resilience** | Automatic circuit breakers, retry policies, and failover pools. |
| **Scalable Architecture** | Decoupled "Brain" (Relay) and "Muscle" (Endpoint) via RabbitMQ. |

## 🛠️ Quick Start

Get your local environment running in minutes.

### 1. Prerequisites

* [Go 1.22+](https://go.dev/)
* [Docker & Compose](https://www.docker.com/)

### 2. Run the Stack

Start the infrastructure (Postgres, Redis, RabbitMQ):

```bash
make dev-up
```

Start the Relay Server:

```bash
go run cmd/relay-server/main.go
```

Start a Worker Endpoint:

```bash
go run cmd/endpoint/main.go
```

👉 **[Read the Full Quick Start Guide](docs/getting-started/quickstart.md)**

## 📚 Documentation

* **[Architecture Overview](docs/architecture/overview.md)**: Deep dive into how it works.
* **[Development Guide](docs/guides/development.md)**: How to build and test.
* **[Contributing](CONTRIBUTING.md)**: Join the community.

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

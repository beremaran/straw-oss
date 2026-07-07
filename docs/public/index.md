# Straw Documentation Portal

Straw is a secure, distributed HTTP/HTTPS egress proxy control plane and worker system. It enables platform teams to route, inspect, validate, throttle, and audit egress request traffic originating from internal workloads before forwarding it to the public internet.

---

## System Architecture

Straw isolates egress traffic control into two distinct operational layers:

1. **Control Plane (`control`)**: Serves the REST API for request forwarding and resource configuration. It evaluates access control, routes requests based on rules, coordinates workload assignments via NATS, stores config in Postgres, manages rate-limits in Redis, and asynchronously writes event logs to ClickHouse.
2. **Egress Workers (`egress`)**: Dedicated worker instances that pull request assignments from NATS, apply TLS fingerprinting profiles, inject authorized headers, enforce tenant deny-rules, execute the HTTP/HTTPS request to the public internet, and stream back the response.

```
                    +--------------------+
                    |   REST API Client  |
                    +--------------------+
                               | (POST /api/v1/requests)
                               v
                    +--------------------+
                    |    Control Plane   | <----> Postgres (Durable Config)
                    +--------------------+ <----> Redis (Transient state, limits)
                               |
                    +---------+---------+
                    |  NATS (Scheduling) |
                    +---------+---------+
                               |
                      +--------+--------+
                      | Egress Worker 1 | ---> Internet (Upstream destination)
                      +-----------------+
```

---

## Core Capabilities

- **Multi-Tenant Configuration**: Dynamic creation of tenants, custom routing rules, header injection policies, deny rules, and quota allocation.
- **Worker Isolation & Security**: Secure Ed25519-signed registration and Redis-backed replay protection for all active Egress workers.
- **Session Stickiness**: Direct related egress requests to the same worker using tenant-supplied session identifiers.
- **Admission Guardrails**: Advanced rate-limiting dimensions and monthly bandwidth/request quota controls.
- **Telemetry & Logs**: Asynchronous, non-blocking telemetry writes to ClickHouse and a Prometheus metrics registry.
- **Connection Reuse**: Optional upstream HTTP connection pooling configured on Egress Workers.

---

## Table of Contents

- [Quickstart Guide](quickstart.md) — Set up a local development cluster using Docker Compose and send your first egress request.
- [Go SDK Guide](sdk.md) — Integrate your Go applications using Straw's native public API client library.
- [CLI Reference](cli.md) — Send requests and manage configuration from the terminal with the `straw` command.
- [Egress Worker Guide](egress_worker.md) — Install, configure, and manage Egress execution worker instances.
- [Authentication & Roles](api/auth.md) — Learn about API key prefixes, scopes, and Role-Based Access Control (RBAC).
- [REST Request Forwarding](api/requests.md) — Documenting the request-forwarding schema, validation normalizations, and canonical error codes.
- [Configuration APIs](api/config.md) — Reference for managing tenants, API keys, worker credentials, pools, routing, and access rules.
- [Telemetry & Observability](api/telemetry.md) — Query ClickHouse logs, trace request attempts, check worker heartbeats, and audit mutations.
- [Runtime Administration](api/admin.md) — Monitor active worker status, globally or tenant-disable workers, and cancel in-flight requests.
- [System Operations](operations.md) — Monitoring readiness, healthchecks, log structure, and current system limits.

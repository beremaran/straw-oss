# Straw Documentation

Straw is a secure, distributed egress proxy control plane and worker system. It allows platform teams to route, inspect, validate, throttle, and audit egress HTTP/HTTPS requests originating from internal workloads before forwarding them to the public internet.

## Architecture & System Overview

Straw separates egress traffic control into two main layers:

1. **Control Plane (`control`)**: Serves the REST API for both request forwarding and resource configuration. It manages authentication, evaluates routing rules, coordinates scheduling/load balancing via NATS, stores configuration in Postgres, coordinates transient state in Redis, and asynchronously flushes metadata to ClickHouse.
2. **Egress Workers (`egress`)**: Dedicated worker instances that pull request assignments via NATS, apply TLS fingerprinting profiles, inject authorized headers, enforce destination deny-lists, execute the HTTP/HTTPS requests to the public internet, and return streamed response payloads.

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

## Features

- **Durable Multi-Tenant Configuration**: Dynamic provisioning of tenants, custom routing rules, fingerprint profiles, header injection policies, deny rules, and quota allocation.
- **Worker Isolation & Replay Protection**: Secure Ed25519-signed registration and Redis-backed replay protection for all active Egress workers.
- **Session Stickiness**: Direct related requests to the same worker using tenant-supplied session identifiers.
- **Admission Guardrails**: Advanced rate-limiting and monthly bandwidth/request quota controls.
- **Change History**: Audit logs tracking all configuration mutations.
- **Telemetry & Metrics**: Asynchronous, non-blocking telemetry writes to ClickHouse and a Prometheus metrics registry.

## Table of Contents

- [Quickstart Guide](quickstart.md) — Set up a local development cluster using Docker Compose and send your first egress request.
- [Authentication & Roles](api/auth.md) — Learn about API key prefixes, scopes, and the system's role-based access control.
- [REST Request forwarding](api/requests.md) — Documenting the request-forwarding schema, validation normalizations, and canonical error codes.
- [Configuration APIs](api/config.md) — Reference for managing tenants, API keys, worker credentials, pools, routing, and access rules.
- [Runtime Administration](api/admin.md) — Monitor active worker status, globally or tenant-disable workers, and cancel in-flight requests.
- [System Operations](operations.md) — Monitoring readiness, healthchecks, log structure, and current system limits.

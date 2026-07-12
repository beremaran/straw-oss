---
sidebar_position: 3
---

# Architecture

Straw has three runtime components:

```text
Application --HTTP--> Control --NATS request/reply--> Egress worker --HTTP/HTTPS--> Destination
                          |
                          +-- health, readiness, Prometheus metrics
```

**Control** exposes the request API, validates authentication and input, applies the deployment policy, selects a
healthy worker, and relays request and response frames. The optional runtime profile also exposes the Admin/Config API
and dashboard. Controls are interchangeable when the Redis runtime-state profile is enabled.

**Egress** registers with Control, advertises capacity, executes outbound requests, and streams responses back. Add
workers when you need more concurrency or network locations.

**NATS** provides discovery, assignment, request/response transport, and runtime snapshot distribution. The default
profile uses Core NATS only. The optional runtime-administration profile enables file-backed JetStream KV for current
configuration and audit history.

**Redis** is optional and stores only expiring coordination state for highly available Control: worker sessions and
heartbeats, capacity, cooldowns, sticky pins, request ownership, cancellation routing, Control instance leases, and
the active configuration version. Redis never stores request or response bodies and is not the durable configuration
authority.

## Request lifecycle

1. A client posts a request to Control.
2. Control validates the bearer token, JSON shape, URL, headers, body size, and timeout.
3. Control selects an available worker from the default pool.
4. The worker acknowledges the assignment and performs the outbound request.
5. Control returns the upstream status, headers, body, and phase timings in one JSON response.

In HA mode, NATS queue subscriptions may deliver a worker registration or heartbeat to any Control. The receiving
instance updates Redis with a session fence and TTL, so every Control routes against the same fleet. A request remains
owned by the Control holding its client connection; the shared owner record lets another instance forward an
administrative cancellation to it over NATS.

GET, HEAD, and OPTIONS requests are replayable by default in the clients. Other methods are not retried unless the
caller explicitly marks them replayable.

## Trust boundary

One Straw deployment is one trust and configuration boundary. Run separate deployments when workloads need isolated
credentials, policy, networks, or operators. This keeps the runtime understandable and avoids hiding a hosted
multi-tenant platform inside the open-source package.

## Source map

- `cmd/control`, `internal/control`: public API and dispatch pipeline
- `cmd/egress`, `internal/egress`: official worker and HTTP executor
- `internal/natsx`: NATS connection and subjects
- `sdk`: Go client and worker SDK
- `python`: Python client and worker SDK
- `deploy/local`: supported development stack
- `deploy/production`: production-pattern example

## 5. Component Boundaries

### Control

Control is stateless apart from in-memory worker and request state. A deployment config file is the source of truth.

Control performs:

- optional deployment-scoped client authentication,
- route evaluation and worker selection,
- NATS request/reply and stream coordination,
- cancellation and request deadlines,
- destination policy resolution,
- final client-facing error mapping,
- Prometheus metrics and structured request logs.

Control does not execute outbound HTTP requests. It does not own tenants, billing, a general-purpose configuration
database, or an analytics warehouse.

### Egress worker

The official Egress worker performs outbound HTTP/HTTPS requests, applies Control-resolved header operations and
supported TLS behavior, enforces destination policy against resolved IPs, enforces deadlines, and reports constrained
execution facts to Control.

Workers hold no control-plane state. Static configuration may include upstream proxy credentials, network-interface
binding, DNS configuration, and local health endpoints.

### Egress SDK and custom workers

The public Egress SDK owns NATS registration, heartbeat, assignment handling, stream framing, and the error protocol
behind a pluggable execution seam. The official worker is its reference implementation.

### NATS

NATS is a transient service transport, not a durable queue, replay log, authorization system, or hidden backlog.
Control chooses the exact worker session for an assignment.

### Optional operator services

Straw exposes Prometheus metrics and structured logs. Storage, dashboards, alerting, TLS termination, and secret
management belong to the operator's environment and appear only as examples or templates in this repository.

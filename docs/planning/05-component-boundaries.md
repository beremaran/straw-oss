## 5. Component Boundaries

### Control

Control is stateful at the policy boundary and mostly stateless at the process boundary. It caches durable config
snapshots but treats Postgres as the source of truth. It uses Redis only for ephemeral runtime state. It writes
operational records to ClickHouse asynchronously.

Control performs:

- API-key authentication,
- RBAC authorization,
- tenant resolution,
- route evaluation,
- worker selection,
- NATS request/reply,
- stream coordination,
- cancellation,
- request deadlines,
- final client-facing error mapping,
- request metadata persistence.

Control does not perform outbound HTTP execution except for later MITM inbound TLS termination and local health/config
endpoints.

### Egress Worker

The official Egress Worker is written in Go. It performs outbound HTTP/HTTPS requests, applies Control-resolved
header/cookie injection, applies supported outbound TLS fingerprint behavior, enforces the request deadline, and reports
constrained execution facts back to Control.

The worker is stateless with respect to Straw control-plane state. Local static config may include upstream proxy
credentials, network-interface binding, DNS configuration, and local health endpoints.

### Egress SDK and Custom Egress Implementations

P2 extracts the worker's protocol machinery — NATS registration, heartbeat, assignment handling, stream framing, and
error protocol — into a public Go Egress SDK with a pluggable execution seam. The official Egress Worker becomes the
reference implementation built on that SDK.

Operators may build custom Egress implementations on the SDK that supply their own execution behavior: forwarding to a
commercial provider, vendor-specific request mechanics, or synthetic responses. Straw does not name or model specific
providers; a provider integration is just a custom Egress implementation.

Custom implementations are operator-configured only, and the operator assumes the executor-side obligations the
official worker enforces: equivalent destination-policy enforcement (see `27-security-controls.md`) and constrained,
public-safe execution facts reported back to Control. There is no marketplace discovery, provider billing
reconciliation, automatic provider account provisioning, or provider economics.

### NATS

P0/P1 use Core NATS only. NATS is a transient service transport, not a durable queue, replay log, tenant authorization
system, or hidden backlog.

NATS queue groups are used only so multiple Control instances can share registration and heartbeat service handling.
Queue groups are not used for executor dispatch. Control chooses the exact executor session.

### Postgres

Postgres is the durable source of truth for tenant and control-plane configuration.

### Redis

Redis stores ephemeral runtime state only. Every Redis key must have a TTL. Redis data loss must not corrupt durable
config. Redis loss may degrade availability decisions, sticky sessions, rate limits, quotas, or certificate caches
depending on explicit fail policy.

### ClickHouse

ClickHouse stores operational analytics, request metadata, worker events, audit records, and P2 payload-capture records.
ClickHouse write failure must not fail request transport.

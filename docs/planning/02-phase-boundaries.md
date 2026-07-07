## 2. Phase Boundaries

The original plan mixed MVP, Phase 1, Phase 2, and mature platform features. This rewrite separates them. A feature is
not considered in-scope for a phase unless it is listed in that phase and has a corresponding contract, schema, or test
row.

### P0 — Vertical Slice

P0 is the first buildable implementation slice.

P0 includes:

- one Control service,
- one official Go Egress Worker,
- externally synchronous REST request transport only,
- generalized API-key authentication (platform-scoped and tenant-scoped via `scope_type`),
- platform API-key lifecycle after bootstrap,
- tenant-scoped API keys and worker credentials,
- tenant-scoped routing rules,
- exact-session Core NATS assignment,
- protobuf Envelope and StreamFrame contracts,
- internal NATS body/response streaming up to configured frame/body limits,
- Control-buffered JSON response envelopes for REST,
- basic worker registration, heartbeat, health, capacity, draining, disable state, duplicate-session handling, and
  cooldown,
- global and tenant-scoped worker admin actions,
- basic rate limits with memory guardrails,
- operational/admission-control quotas with explicit non-billing-grade accuracy limits,
- destination deny rules with Control-side URL/host validation and Egress-side resolved-IP validation,
- Control-resolved destination policy bundles sent to Egress per request,
- explicit header injection operations resolved by Control; no arbitrary live traffic mutation,
- fingerprint profile selection with P0 profiles limited to the worker-supported set,
- canonical ErrorResponse envelope,
- request metadata written asynchronously to ClickHouse with P0 metadata redaction rules,
- local docker-compose environment,
- table-driven unit tests and E2E tests for the P0 flow.

P0 excludes:

- HTTP forward proxy,
- raw CONNECT,
- MITM,
- the Egress SDK and custom Egress implementations,
- object-storage large-body transport,
- direct streaming large-body transport,
- external REST response streaming,
- payload capture,
- capture hints other than `none`,
- redirect following,
- SDKs beyond a minimal generated/prototype client,
- CLI/UI beyond basic admin smoke tooling,
- Kubernetes/Swarm production manifests,
- HTTP/2 semantics,
- upstream keep-alive pooling and advanced/shared upstream connection-pool management.

P0 Egress disables upstream HTTP keep-alives and outbound HTTP/2 by default. Those may be re-enabled only behind an
explicit tested feature flag in a later phase.

### P1 — Proxy Transport and Operational Hardening

P1 adds:

- HTTP forward proxy,
- raw CONNECT tunnel mode,
- worker-loss and NATS-outage hardening beyond the P0 baseline,
- richer config-management APIs,
- SDK, CLI, and minimal UI surfaces,
- telemetry read APIs and dashboards,
- Control-aggregated Egress metrics behind an explicit enablement flag,
- optional upstream connection pooling if explicitly specified and tested,
- improved observability dashboards,
- load and backpressure testing,
- operational deployment templates.

### P2 — MITM, Large Bodies, Payload Capture, Egress SDK

P2 adds:

- MITM HTTPS decoded mode,
- generated leaf certificate cache/storage,
- BodyRef transport after choosing the P2 response-body mode,
- payload capture with storage-only redaction,
- the public Egress SDK, the official worker rebased onto it, and one example custom Egress implementation,
- HTTP/2 support where explicitly specified and tested,
- quota reconciliation suitable for billing-grade or near-billing-grade reporting if required.

### Future Work

Future work includes:

- WebSockets,
- SOCKS5,
- generic TCP/UDP/QUIC,
- scraping orchestration,
- persistent request queues,
- automatic retries/replay workflows,
- public marketplace workflows,
- billing/payment workflows,
- active egress verification,
- plugin runtime inside workers,
- managed disaster recovery.

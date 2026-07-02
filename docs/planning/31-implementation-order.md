## 31. Implementation Order

### P0

1. Repository scaffolding, config loader, schema validation, generated protobuf.
2. Canonical `straw.v1` protobuf, StreamFrame sequencing, DestinationPolicy, ErrorResponse details, and Buf CI.
3. NATS connection, max-payload startup validation, and exact-session subject protocol.
4. Postgres schema for tenants, platform/tenant roles, API keys, worker credentials, pools, routes, deny rules,
   injection policies, rate limits, quotas, config versions, and audit source records.
5. Config snapshot cache with Postgres versioning and Redis invalidation.
6. Control REST `/api/v1/requests` minimal non-streaming transport endpoint.
7. API-key authentication, platform/tenant API key lifecycle, tenant resolution, RBAC, and cache invalidation for
   revocation.
8. Worker registration, heartbeat, state machine, duplicate-session handling, global and tenant worker admin state,
   draining, disable, and cooldown.
9. Routing snapshot evaluation, tenant isolation, worker eligibility, degraded-worker policy, sticky sessions.
10. Assignment and stream frame lifecycle with sequence/offset/credit validation.
11. Official Go Egress outbound request execution with P0 transport defaults, deadline enforcement, and
    DestinationPolicy
    resolved-IP enforcement.
12. Canonical error registry and ErrorResponse mapping.
13. Redis rate limits, quota hot counters, worker state, sticky sessions, and explicit Redis failure policies.
14. ClickHouse request metadata write path with redaction/sanitization.
15. P0 test matrix and docker-compose.

### P1

1. HTTP forward proxy.
2. Raw CONNECT tunnel.
3. SDK/CLI minimal surfaces.
4. Telemetry read APIs and UI minimal admin/observability surface.
5. Optional direct worker Prometheus scraping if chosen.
6. Optional upstream connection pooling if explicitly designed.
7. Load/backpressure hardening.
8. Production deployment templates.

### P2

1. MITM decoded HTTPS.
2. Large-body BodyRef transport.
3. Payload capture.
4. Provider Adapter protocol and static adapter.
5. HTTP/2 support if fully specified and tested.
6. Billing-grade or near-billing-grade quota reconciliation if required.

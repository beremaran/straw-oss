# PLAN Evaluation #1

## Consistency Issues

**1. NATS subject naming mismatch (Critical)**
Section 10 defines subjects as `straw.v1.control.register`, `straw.v1.control.heartbeat`, `straw.v1.executor.<worker_id>.<session_id>.assign`, etc. Section 20.4.2's topology table uses completely different patterns: `straw.v1.register.>`, `straw.v1.heartbeat.{worker_id}`, `straw.v1.dispatch.{pool_name}`, `straw.v1.stream.c2e.{request_id}`. These are structurally incompatible — one uses `control.` prefix and queue groups, the other uses wildcard and stream subjects. This needs to be unified.

**2. "No fixed architectural limits" vs hardcoded defaults**
Section 2 goals state "Avoid fixed architectural limits on request concurrency or request rate." But the config examples (Sections 20.3.1–20.3.3) hardcode `max_open_conns: 20`, `buffer_size_mb: 32`, `max_conns: 10`, `max_ping_failures: 3`, etc. These are operational defaults, not architectural limits, but the distinction isn't called out. The goals should clarify these are tunable defaults, or the config sections should explicitly note they're all overridable.

**3. Rate limit algorithm naming conflict**
Section 2 goals say "sliding window" for rate limits. Section 17 says "Redis Sliding Window Log using Sorted Sets" for rate limits but "Redis Fixed Window Counters (INCRBY) with a 30-day TTL" for quotas. The quota mechanism is fine (fixed window makes sense for monthly resets), but the terminology should be clearer to avoid confusion between the two sliding vs fixed window approaches.

**4. MITM cert storage chain is inconsistent**
Section 13 says certs are "written to disk by default" with S3 as a configurable backend. Section 15 says Redis uses `volatile-lru` and cache misses "fetch it from S3." Section 24 says "falls back to durable S3-compatible storage." The three sections describe a coherent 3-tier strategy (Redis → disk → S3), but the disk tier is missing from Section 15's scope list and Section 24's fallback description skips disk entirely.

**5. Error code inconsistency**
Section 10 mentions `transport_unavailable` as the external mapping for NATS failures. Section 18.3's error registry has `nats_cluster_unavailable` (code 202) but no `transport_unavailable`. These need to be the same code.

---

## Coverage Gaps

**6. Provider Adapter lifecycle is thin**
Provider Adapters appear in Section 9 (worker discovery), Section 10 (NATS protocol), Section 11 (protobuf), and Section 20.3.3 (static config), but there's no dedicated section explaining their lifecycle: how they receive assignments, how they proxy to third-party providers, how they report health, and how errors map. Section 14 (Egress Execution) only covers Egress Workers, not Adapters.

**7. REST API request/response schemas are absent**
Section 6 describes the API surface (URLs and methods) but doesn't define the request/response JSON schemas for the transport endpoint. The protobuf types are defined, but the REST wire format (JSON envelope structure, field names, pagination, etc.) is not specified.

**8. CLI and UI sections are placeholders**
Section 21 mentions "Phase 1 client SDKs, CLI, and UI" but the section content is minimal. No SDK language targets, no CLI command list, no UI framework or screens. The goals (Section 2) promise these as Phase 1 deliverables, so they need at least an outline.

**9. Config management API is missing worker/admin endpoints**
Section 20.12 defines endpoints for tenants, API keys, and routing rules, but there's no endpoint for managing worker credentials, admin disable flags, or payload capture policies — all of which are mentioned as admin actions in Sections 9 and 16.

**10. `config_version` field has no config examples**
Section 20.10 describes `config_version` schema versioning with rejection and migration logic, but none of the config YAML examples (Sections 20.3.1–20.3.3) show this field.

**11. MITM CA distribution endpoint not in API surface**
Section 13 says Control "exposes a dedicated HTTP endpoint to distribute the public CA certificate," but this endpoint doesn't appear in Section 6 (REST API), Section 12 (ingress modes), or Section 20.12 (config API).

---

## Completeness Issues

**12. Implementation order omits SDKs, CLI, UI, and Adapters**
Section 26 lists only: (1) Protobuf + NATS, (2) Control Server, (3) Egress Worker. It skips Provider Adapters (which are a first-class component), Client SDKs, CLI, and UI — all called out in Section 2 goals. The order should be at least 5 steps.

**13. Worker loss handling is described but not tested**
Section 9 describes worker loss fallback behavior (reroute attempts, deadline enforcement). Section 23.2 E2E tests list "worker loss" as a check, but the test specification is vague — it should describe the exact scenario (e.g., "kill worker mid-stream for non-idempotent POST, verify 502 with `worker_disconnected`").

**14. Request ID lifecycle is underspecified**
`request_id` appears everywhere (NATS subjects, protobuf envelope, error response, logs, metrics), but the plan never answers: who generates it (client or Control)? Can clients supply their own? How does it survive retries and fallbacks?

**15. Tenant isolation in routing is not described**
Section 2 goals promise "preserve tenant isolation." Sections 7–8 describe routing but never mention that routing rules, worker pools, and capabilities are scoped per tenant. A routing rule for `tenant-A` must never route to a `tenant-B` worker. This isolation mechanism is missing.

**16. Large-body transport implementation details are scattered**
`BodyRef`, S3 references, direct streaming, and `large_body_threshold_bytes` appear in Sections 10, 11, and 20.3, but there's no unified description of the flow: when does Control switch to large-body mode? Who uploads to S3? How does the executor download? What happens if S3 is down during streaming?

**17. NATS auth scope is mentioned but not specified**
Section 10 says "Control, workers, and adapters receive credentials scoped to their allowed subject prefixes." But no subject allowlists or denylists are defined. What subjects can workers publish to? What can Control? This is critical for the security model.

**18. No discussion of graceful shutdown sequencing**
Section 9 mentions "graceful shutdown" for workers and Section 2 goals mention "graceful shutdown" for Control. But there's no shutdown sequence: does Control stop accepting new requests before draining NATS subscriptions? How long is the drain window? What happens to in-flight requests?

**19. `trace_id` propagation across layers is incomplete**
Section 19 mentions W3C trace context headers (`traceparent`, `tracestate`) and propagation. But the NATS envelope uses `trace_id` as a protobuf field, while the REST API uses HTTP headers. The plan doesn't specify how these map — does Control extract `traceparent` from the inbound HTTP request and populate the NATS envelope `trace_id`?

**20. ClickHouse CDC sync from Postgres is a dependency not described**
Section 15 mentions Postgres data is "synced to ClickHouse via CDC or ClickHouse Dictionaries." No tool (Debezium? materialized views? custom pipeline?) is specified, and there's no section on how this sync is maintained or what happens during a Postgres-to-ClickHouse lag.

---

## Minor / Nits

- Section 11 says "proto3 and no `required` fields" but also says "Receivers validate mandatory business fields after decoding." This is fine proto3 practice, but the validation behavior should be called out as a pattern, not just mentioned once.
- Section 18.1's `ErrorResponse` has `upstream_status` (field 7) described as "Origin HTTP status (only if egress_involved)" but `TimeoutType` (field 8) is only for timeout errors. These conditional fields should have a note about when they're zero vs absent.
- Section 20.11.1 docker-compose maps 5 ports on Control (8080–8084) plus metrics (9090), but Section 6 only describes 4 entrypoints (REST, HTTP proxy, CONNECT, MITM). The port mapping should be explicit: which port serves which entrypoint.
- Section 16.2 mentions "cryptographically signed tokens" but doesn't specify the algorithm (Ed25519? RSA? ECDSA?) or key distribution mechanism.

---

## Summary

| Dimension | Issues | Severity |
|-----------|--------|----------|
| **Consistency** | 5 | 1 critical (NATS subjects), 4 medium |
| **Coverage** | 6 | 6 medium — mostly missing detail in well-defined areas |
| **Completeness** | 10 | 3 medium, 7 minor — many are "described but not fleshed out" |

The plan is thorough for what it covers (MITM design, error taxonomy, storage boundaries, config schema, observability). The highest-priority fix is **unifying the NATS subject definitions** between Sections 10 and 20.4 — everything built on top of that contract will be wrong if the two sections don't match.

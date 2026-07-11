## 32. Open Decisions

These must be decided before related implementation starts. Defaults already specified elsewhere are not open decisions.

### P1 REST Streaming Response Format — Resolved 2026-07-06

- **Blocked sections**: Section 7 REST Streaming Variant, Section 30 P1 tests.
- **Decision**: Binary framing, as specified in Section 7 REST Streaming Variant.
- **Rejected options**: HTTP chunked JSON events; server-sent events.
- **Acceptance tests required**: metadata before body bytes, upstream error after partial body, client cancellation, body
  limit behavior, and trailer handling.
- **Decision owner**: Control/API owner.

### P1 Egress Metrics Exposure — Resolved 2026-07-06

- **Blocked sections**: Section 23 Egress Metrics, Section 28 deployment ports.
- **Decision**: Control-aggregated metrics only, behind an explicit enablement flag.
- **Rejected options**: Direct worker Prometheus scrape; both with explicit deployment flag.
- **Acceptance tests required**: metric cardinality limits, flag disabled/enabled behavior, no worker-local
  endpoint/port exposure, and outage behavior when Control cannot aggregate.
- **Decision owner**: Operations owner.

### P1 Multiple Concurrent Control Replicas — Resolved 2026-07-07

- **Blocked sections**: `../implementation-history.md#p1-23` (its gate), Section 21
  runtime-state placement of cross-instance in-flight state.
- **Decision**: Straw supports running more than one Control replica sharing one request plane. Cross-instance
  runtime coordination lives in the existing Redis runtime-state tier (Section 21), not a new datastore, and is
  gated per deployment by an explicit enablement flag (`server.multi_control_enabled`, default off) so a
  single-Control deployment pays no extra Redis round-trips.
- **Rejected options**: single-Control-only forever; a new dedicated coordination datastore; always-on
  cross-instance state with no flag.
- **Acceptance tests required**: cross-instance admin cancel tears down a request in-flight on a sibling replica;
  the single-Control fast path still cancels in-process without touching Redis; an unknown `request_id` still
  returns the existing not-found outcome.
- **Decision owner**: Operations owner.

### P2 MITM Private-Key Storage Policy — Resolved 2026-07-07

- **Blocked sections**: Section 17 MITM Design, Section 27 Security Controls.
- **Decision**: KMS-backed shared cache. Generated leaf cert bundles (including private keys) are stored in a shared
  cache encrypted via a KMS-compatible mechanism, tenant/deployment scoped, readable by Control instances only.
- **Rejected options**: never store generated leaf private keys; encrypted Redis/disk cache with a deployment key.
- **Acceptance tests required**: cache miss generation, encrypted-at-rest verification for stored keys, rotation, and
  unique-SNI flood limits.
- **Decision owner**: Security owner.

### P2 BodyRef Response-Body Mode — Resolved 2026-07-07

- **Blocked sections**: Section 18 Large-Body Transport, Section 13 BodyRef protobuf usage.
- **Decision**: Executor streams the response body through Control while teeing to object storage. Streaming remains
  the synchronous transport path; the teed object backs REST download references and retention policy.
- **Rejected options**: executor writes the response object and Control reads after completion.
- **Acceptance tests required**: cancellation cleanup, checksum/size validation, object retention, object-storage outage,
  and response-body-too-large behavior.
- **Decision owner**: Transport owner.

### P2 Provider Adapter Baseline — Superseded 2026-07-07

- **Blocked sections**: Section 5 Egress SDK, Section 31 P2 implementation order.
- **Decision**: Drop the Provider Adapter concept entirely. P2 ships a public Egress SDK that owns the NATS
  registration, heartbeat, assignment, stream, and error protocol behind a pluggable execution seam; the official
  Egress Worker is rebased onto the SDK as its reference implementation; and one example custom Egress implementation
  proves the SDK end to end. Provider integrations are just custom Egress implementations — Straw names no providers.
  Custom implementations remain operator-configured only, with no marketplace or provider billing behavior.
- **Rejected options**: a separate Provider Adapter entity and protocol; a named static provider adapter (the earlier
  same-day resolution: adapter protocol plus one static Bright Data adapter); protocol scaffolding only.
- **Acceptance tests required**: SDK-built worker protocol conformance (registration, assignment, stream, errors),
  official worker on the SDK passing the existing E2E flow, constrained error facts, and no marketplace/provider
  billing behavior.
- **Decision owner**: Integrations owner.

### P2 Quota Reconciliation Accuracy — Resolved 2026-07-07

- **Blocked sections**: Section 20 Reconciliation Position, Section 33 Quota Accuracy.
- **Decision**: Billing-grade accounting. Reconciliation must define a durable usage-event source, aggregation cadence,
  idempotency keys, late-arriving event handling, a correction policy for Redis hot counters, and user-visible quota
  display semantics accurate enough to invoice against.
- **Rejected options**: operationally accurate reconciliation; near-billing-grade reconciliation.
- **Acceptance tests required**: idempotent aggregation, late event handling, correction policy, and user-visible quota
  display semantics.
- **Decision owner**: Billing/operations owner.

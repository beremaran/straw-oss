## 32. Open Decisions

These must be decided before related implementation starts. Defaults already specified elsewhere are not open decisions.

### P1 REST Streaming Response Format

- **Blocked sections**: Section 7 REST Streaming Variant, Section 30 P1 tests.
- **Options**: HTTP chunked JSON events; binary framing; server-sent events.
- **Acceptance tests required**: metadata before body bytes, upstream error after partial body, client cancellation, body
  limit behavior, and trailer handling.
- **Decision owner**: Control/API owner.

### P1 Egress Metrics Exposure

- **Blocked sections**: Section 23 Egress Metrics, Section 28 deployment ports.
- **Options**: Control-aggregated metrics only; direct worker Prometheus scrape; both with explicit deployment flag.
- **Acceptance tests required**: metric cardinality limits, worker-local endpoint access control, and outage behavior when
  Control cannot aggregate.
- **Decision owner**: Operations owner.

### P2 MITM Private-Key Storage Policy

- **Blocked sections**: Section 17 MITM Design, Section 27 Security Controls.
- **Options**: never store generated leaf private keys; encrypted Redis/disk cache; KMS-backed shared cache.
- **Acceptance tests required**: cache miss generation, encrypted-at-rest verification for stored keys, rotation, and
  unique-SNI flood limits.
- **Decision owner**: Security owner.

### P2 BodyRef Response-Body Mode

- **Blocked sections**: Section 18 Large-Body Transport, Section 13 BodyRef protobuf usage.
- **Options**: executor writes response object and Control reads after completion; executor streams through Control while
  teeing to object storage.
- **Acceptance tests required**: cancellation cleanup, checksum/size validation, object retention, object-storage outage,
  and response-body-too-large behavior.
- **Decision owner**: Transport owner.

### P2 Provider Adapter Baseline

- **Blocked sections**: Section 5 Provider Adapter, Section 31 P2 implementation order.
- **Options**: protocol scaffolding only; one static Bright Data adapter.
- **Acceptance tests required**: adapter registration, destination-policy enforcement, constrained error facts, and no
  marketplace/provider billing behavior.
- **Decision owner**: Integrations owner.

### P2 Quota Reconciliation Accuracy

- **Blocked sections**: Section 20 Reconciliation Position, Section 33 Quota Accuracy.
- **Options**: operationally accurate reconciliation; near-billing-grade reconciliation; billing-grade accounting.
- **Acceptance tests required**: idempotent aggregation, late event handling, correction policy, and user-visible quota
  display semantics.
- **Decision owner**: Billing/operations owner.

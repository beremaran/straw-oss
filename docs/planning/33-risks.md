## 33. Risks

### Contract Drift

The largest implementation risk is reintroducing competing contracts. NATS/protobuf, error codes, routing semantics,
Postgres config schemas, and ClickHouse schema must each have one canonical section.

### Control CPU Saturation

MITM TLS termination, certificate generation, JSON REST encoding, and high-cardinality route evaluation can saturate
Control. P0 avoids MITM and uses cached route snapshots. P2 must add explicit concurrency/rate controls for certificate
generation.

### CGO/Outbound TLS Bottlenecks

Outbound TLS fingerprint libraries may block or leak resources. Egress must isolate CGO/FFI and enforce deadlines.

### Quota Accuracy

Redis-only quota counters are not durable. P0 quotas are operational admission controls. Section 32 selects
billing-grade P2 reconciliation, so task `docs/tasks/p2/17-quota-reconciliation.md` must define and test the durable
usage-event source, aggregation, idempotency, late-event handling, corrections, and user-visible display semantics.

### Config Staleness

Redis pub/sub invalidation can be missed. Postgres tenant config versions are the durable source of truth, and Control
must periodically or synchronously detect newer versions for new requests.

### SSRF/Destination Abuse

Deny rules must run both before routing and after DNS resolution. Egress-side resolved-IP enforcement is mandatory and
must use the per-request DestinationPolicy bundle sent by Control. Upstream proxy mode introduces additional risk:
the proxy performs DNS resolution and connection establishment, so the worker cannot prove the resolved-IP policy.
Deployment must explicitly trust the proxy for equivalent SSRF enforcement.

### Metadata Leakage

Even without payload capture, full URLs, headers, and error details can leak secrets. P0 metadata redaction is mandatory
for logs and ClickHouse writes. Path metadata may contain secrets (tokens, signed IDs) and should be treated as
sensitive telemetry.

### NATS Payload Limits

Frame sizes that fit the Straw config may still exceed the NATS server max payload after protobuf envelope overhead.
Startup validation must fail unsafe configurations.

### NATS Subscription Race

Core NATS does not retain messages for later subscribers. If `RequestStart` is published before the executor's
subscription is registered, the frame is lost. The assignment flow enforces subscription-before-publish ordering
with explicit flush calls.

### Payload Capture Liability

Payload capture can store sensitive data. It must be explicit, bounded, redacted only for stored copies, and off by
default.

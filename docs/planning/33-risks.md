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

Redis-only quota counters are not durable. P0 quotas are operational admission controls. Billing-grade accounting
requires a later reconciliation design or an explicitly implemented P0 reconciliation job.

### Config Staleness

Redis pub/sub invalidation can be missed. Postgres tenant config versions are the durable source of truth, and Control
must periodically or synchronously detect newer versions for new requests.

### SSRF/Destination Abuse

Deny rules must run both before routing and after DNS resolution. Egress-side resolved-IP enforcement is mandatory and
must use the per-request DestinationPolicy bundle sent by Control.

### Metadata Leakage

Even without payload capture, full URLs, headers, and error details can leak secrets. P0 metadata redaction is mandatory
for logs and ClickHouse writes.

### NATS Payload Limits

Frame sizes that fit the Straw config may still exceed the NATS server max payload after protobuf envelope overhead.
Startup validation must fail unsafe configurations.

### Payload Capture Liability

Payload capture can store sensitive data. It must be explicit, bounded, redacted only for stored copies, and off by
default.

# Appendix A — Reconciliation Notes

This rewrite intentionally makes these replacements and hardening changes:

- Replaces pool queue-group NATS dispatch with exact-session dispatch.
- Replaces duplicate ClickHouse schemas with one canonical `straw.*` table set.
- Replaces `sub-millisecond` public routing claims with p50/p99 SLOs.
- Replaces live payload redaction with storage-only capture redaction.
- Removes `upstream_http_error` as a Straw error for normal origin statuses.
- Separates Control fallback from SDK/client retry.
- Removes default replayability for `PUT` and `DELETE`.
- Moves MITM, BodyRef, Provider Adapters, and payload capture out of P0.
- Fixes `STROW_` environment variable typo class.
- Uses `/api/v1` as the single public API base path.
- Defines `requester` as the data-plane execution role.
- Treats inbound TLS termination as server-side TLS, not outbound `tls-client` behavior.
- Defines generated per-SNI certificates as leaf certificates, not intermediates.
- Adds platform-scoped `system_admin` and tenant-scoped `tenant_admin` to remove tenant-creation RBAC ambiguity.
- Defines P0 REST as externally non-streaming while allowing internal NATS DataFrame streaming.
- Uses `body_too_large` with `details.direction` instead of an undefined `response_body_too_large` code.
- Moves worker runtime operations from `/api/v1/config/*` to `/api/v1/admin/*`.
- Adds StreamFrame sequencing, DataFrame offsets, and `OutboundStartFrame`.
- Qualifies the terminal-frame invariant for worker/NATS loss and synthetic terminal outcomes.
- Defines P0 fallback as conservative after `RequestStart` unless `replayable=true`.
- Adds per-request `DestinationPolicy` so Egress can enforce resolved-IP policy without querying Control stores.
- Adds canonical P0 Postgres model and config resource schemas.
- Adds durable tenant config-version checks so Redis pub/sub is not the only invalidation path.
- Adds P0 metadata redaction rules for URLs, auth headers, cookies, logs, and ClickHouse.
- Adds NATS max-payload startup validation.
- Resolves HTTP/2 as P2 unless explicitly moved later.
- Resolves object-storage body retention default as 1 day, configurable up to 3 days for debugging.
- Defines P0 quotas as operational admission controls rather than billing-grade accounting.

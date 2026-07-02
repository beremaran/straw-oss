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

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

## Audit Reconciliation — 2026-07-03

A pre-implementation audit applied these corrections and decisions:

- Requires NATS server `max_payload` ≥ `max_frame_data_bytes + 65536`; local compose ships
  `deploy/docker/nats-server.conf` with `max_payload: 2MB` because the stock 1 MiB default fails startup validation
  against the 1 MiB default frame size.
- Removes the undefined internal protobuf `Error` message: executors emit canonical `ErrorCode` in `ErrorFrame` with
  the low-level fact in `details["fact"]`; Control validates codes against an executor-emittable set and treats
  out-of-set codes as protocol violations.
- Restricts P0 worker-credential creation to the caller's tenant; multi-tenant credentials become a platform-scoped
  P1 operation.
- Makes quota configs platform-managed (`system_admin` via `PUT /tenants/{id}/quotas`); rate limits stay
  tenant-managed under an optional `system_admin`-set per-tenant `rate_limit_ceiling`.
- Declares P0 fingerprint profiles seeded built-ins with no write API and removes the operator fingerprint-mutation
  grant; tenant-authored profiles are P1.
- Adds reply-inbox prefixes (`_INBOX.ctl.>`, `_INBOX.wrk.<worker_id>.>`) to the NATS ACL table so request/reply works
  under scoped permissions.
- Adds `GET /api/v1/admin/workers` (platform: all workers; tenant-scoped: eligible workers only, no session IDs) and
  a tenant-ownership check plus `system_admin` access on request cancellation.
- Adds Control `/healthz` and `/readyz` on the metrics port.
- Defines the sticky-session Redis key as `straw:sticky:<tenant_id>:<sticky_session_id>` (tenant-scoped, TTL from the
  matched rule, re-pinned on permitted fallback).
- Sets `api_keys.config_version` to 0 for platform keys.
- Keeps `control.server.metrics_port` and removes `control.observability.metrics.host/port`; completes the canonical
  config-key table with the egress capability/observability and control tracing keys.
- Records the Buf lint exceptions (`ENUM_VALUE_PREFIX`, `ENUM_ZERO_VALUE_SUFFIX`) as deliberate: `SniHostMismatchPolicy`
  and `RedirectPolicy` use the safest behavior as their zero value.
- Deduplicates the credit-exhaustion paragraph in Section 12 and makes Section 15 the single canonical home of the
  header-injection safety table (Section 27 now references it).

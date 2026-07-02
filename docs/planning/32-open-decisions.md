## 32. Open Decisions

These must be decided before related implementation starts:

1. REST successful response streaming format for P1 `/api/v1/requests:stream`.
2. Whether P1 Egress exposes Prometheus metrics directly or only reports to Control.
3. P2 MITM private-key storage policy.
4. P2 BodyRef response-body mode.
5. Whether Provider Adapter ships with a real Bright Data adapter or only protocol scaffolding.
6. Whether P2 quota reconciliation must be billing-grade or only operationally accurate.

Resolved by this rewrite:

- P0 REST is externally non-streaming and internally may use NATS DataFrames.
- P0 uses `body_too_large` for request and response body limit failures.
- P0 rejects `CONNECT` in REST transport.
- P0 rejects `capture_hint` values other than `none`.
- P0 disables redirect following.
- P0 disables outbound HTTP/2 and upstream keep-alives by default.
- HTTP/2 is P2 unless moved by an explicit future decision.
- Object-storage body retention default is 1 day, configurable up to 3 days for debugging.
- P0 quotas are operational admission controls, not billing-grade durable accounting.
- Worker runtime actions live under `/api/v1/admin/*`, not `/api/v1/config/*`.
- Tenant creation requires `system_admin`; tenant-local administration uses `tenant_admin`.

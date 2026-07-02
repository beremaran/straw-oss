## 32. Open Decisions

These must be decided before related implementation starts:

1. REST successful response streaming format for P1 `/api/v1/requests:stream`.
2. Whether P1 Egress exposes Prometheus metrics directly or only reports to Control.
3. P2 MITM private-key storage policy.
4. P2 BodyRef response-body mode.
5. Whether Provider Adapter ships with a real Bright Data adapter or only protocol scaffolding.
6. Whether P2 quota reconciliation must be billing-grade or only operationally accurate.
7. Upstream proxy remote resolution is not trusted by default. It must be explicitly enabled by deployment or tenant policy.
8. Specific egress timeout defaults (`connect_timeout_ms`, `response_header_timeout_ms`, `upload_idle_timeout_ms`,
    `download_idle_timeout_ms`).

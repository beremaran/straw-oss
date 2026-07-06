## 23. Observability

### Metrics

Control exposes Prometheus metrics at `/metrics`.

P0 metrics:

- `straw_requests_total`,
- `straw_request_duration_seconds`,
- `straw_routing_duration_seconds`,
- `straw_assignment_duration_seconds`,
- `straw_active_requests`,
- `straw_worker_sessions`,
- `straw_workers_available`,
- `straw_worker_heartbeat_age_seconds`,
- `straw_nats_request_duration_seconds`,
- `straw_nats_errors_total`,
- `straw_clickhouse_write_queue_depth`,
- `straw_clickhouse_write_errors_total`,
- `straw_rate_limit_rejections_total`,
- `straw_quota_rejections_total`.

Label cardinality must be controlled. Do not label high-cardinality URLs directly in Prometheus. Use `target_host`,
`tenant_id`, `route_id`, and `error_code`; full URLs belong in ClickHouse/logs.

### Egress Metrics

P1 uses Control-aggregated Egress metrics only, behind an explicit enablement flag. Egress reports bounded telemetry to
Control over the service boundary; Control exposes the resulting Prometheus series on its metrics surface.

Direct worker Prometheus scraping is not a shipped P1 mode. Do not expose a worker-local `/metrics` endpoint or map a
worker metrics port for Egress metrics unless a later task explicitly reopens that decision.

### Logs

All services emit structured JSON logs with:

- `service`,
- `timestamp`,
- `level`,
- `request_id` where available,
- `tenant_id` where available,
- `trace_id` where available,
- `error_code` where available,
- `worker_id` only in internal logs.

### SLOs

Control-side routing/coordination target is:

- p50 < 100 ms,
- p99 < 500 ms.

Do not claim sub-millisecond routing as a system guarantee. Sub-millisecond route evaluation may be an internal
optimization target for cached snapshots, but the public SLO is p99 < 500 ms excluding outbound execution.

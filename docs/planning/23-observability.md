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

Choose one of the following per deployment:

- Control-aggregated metrics only: Egress reports telemetry over NATS.
- Direct worker scrape: Egress exposes `/metrics` locally.

P0 should prefer direct local `/healthz` and `/readyz`, with Control-aggregated request outcomes. If direct Prometheus
scrape is shipped, document it as an explicit supported path.

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

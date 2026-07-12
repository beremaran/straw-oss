---
sidebar_position: 9
---

# Operations

## Health and readiness

Control exposes these endpoints on its metrics port (default `9090`):

- `/healthz`: process liveness;
- `/readyz`: service readiness, including Redis reachability in HA mode;
- `/metrics`: Prometheus text format.

Egress exposes `/healthz` and `/readyz` on its health port (default `8090`).

## Metrics

Prometheus metrics use bounded labels. Key series include:

- `straw_requests_total{error_code}`;
- `straw_request_duration_seconds`;
- `straw_active_requests`;
- `straw_routing_duration_seconds`;
- `straw_assignment_duration_seconds`;
- `straw_nats_request_duration_seconds` and `straw_nats_errors_total`;
- `straw_worker_sessions`, `straw_workers_available`, and `straw_worker_heartbeat_age_seconds`.
- `straw_runtime_state_available`, `straw_runtime_state_operations_total`, and `straw_runtime_state_errors_total`
  when Redis coordination is enabled.
- `straw_receipts_created_total`, `straw_receipt_parts_uploaded_total`, `straw_receipts_verified_total`,
  `straw_receipts_rejected_total`, `straw_receipt_assignments_total`, `straw_receipts_consumed_total`, and
  `straw_receipts_expired_total` when object storage is enabled.

Expose metrics only to your monitoring network. No ClickHouse or telemetry database is required.

## Logs

Control and Egress write structured JSON to stdout. Collect container stdout with the logging system already used by
your environment. Request IDs connect client errors, Control logs, and worker activity.

## Scaling and shutdown

Scale Egress workers for outbound concurrency. A single Control is the simplest deployment. For Control HA, place at
least two Redis-backed Controls behind a readiness-aware load balancer. On SIGTERM, Control fails readiness and gives
active HTTP requests up to the configured maximum request timeout plus five seconds to finish before draining NATS.

## Upgrades

Read `CHANGELOG.md`, test the new version against a staging deployment, update workers before or with Control, and
keep protocol-coupled custom workers pinned until verified.

When the runtime-administration profile is enabled, include the NATS JetStream configuration bucket in backup and
recovery drills. Inspect rollout status after upgrades; an official worker reports `applied` after receiving the
current snapshot. See [Runtime administration](runtime-administration.md#back-up-and-recover).

When receipt storage is enabled, back up durable receipt records and verified bodies according to their explicit
retention, monitor rejection/expiry counters, and test cleanup plus interrupted-upload recovery. The S3 bucket or
local volume is optional application data; NATS and Redis never contain receipt bodies.

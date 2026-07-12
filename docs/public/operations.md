---
sidebar_position: 9
---

# Operations

## Health and readiness

Control exposes these endpoints on its metrics port (default `9090`):

- `/healthz`: process liveness;
- `/readyz`: NATS and service readiness;
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

Expose metrics only to your monitoring network. No ClickHouse or telemetry database is required.

## Logs

Control and Egress write structured JSON to stdout. Collect container stdout with the logging system already used by
your environment. Request IDs connect client errors, Control logs, and worker activity.

## Scaling and shutdown

Scale Egress workers for outbound concurrency. A single Control is the simplest deployment; place a reverse proxy in
front if you need TLS and connection management. Services handle SIGTERM, stop accepting new work, and drain NATS
connections during normal shutdown.

## Upgrades

Read `CHANGELOG.md`, test the new version against a staging deployment, update workers before or with Control, and
keep protocol-coupled custom workers pinned until verified.

When the runtime-administration profile is enabled, include the NATS JetStream configuration bucket in backup and
recovery drills. Inspect rollout status after upgrades; an official worker reports `applied` after receiving the
current snapshot. See [Runtime administration](runtime-administration.md#back-up-and-recover).

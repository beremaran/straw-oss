# Operations

## Health and readiness

Control exposes these endpoints on its metrics port (default `9090`):

- `/healthz`: process liveness;
- `/readyz`: service readiness, including Redis reachability in HA mode;
- `/metrics`: Prometheus text format.

Egress exposes `/healthz` and `/readyz` on its health port (default `8090`).

## Metrics

Prometheus metrics use bounded labels. Counters are cumulative process-lifetime values; gauges are instantaneous.
`none` means the metric has no variable labels.

| Metric | Type | Labels | Unit | Profile | Interpretation |
| --- | --- | --- | --- | --- | --- |
| `straw_requests_total` | counter | `error_code` | requests | all | completed Control requests; empty code is success |
| `straw_request_duration_seconds` | histogram | none | seconds | all | end-to-end Control request latency |
| `straw_active_requests` | gauge | none | requests | all | requests currently executing in this Control |
| `straw_routing_duration_seconds` | histogram | none | seconds | all | worker-selection latency |
| `straw_assignment_duration_seconds` | histogram | none | seconds | all | assignment request/ack latency |
| `straw_nats_request_duration_seconds` | histogram | none | seconds | all | NATS request/reply latency |
| `straw_nats_errors_total` | counter | `error_code` | errors | all | NATS transport failures by stable error code |
| `straw_worker_sessions` | gauge | none | sessions | all | registered live worker sessions |
| `straw_workers_available` | gauge | none | workers | all | workers currently eligible for assignment |
| `straw_worker_heartbeat_age_seconds` | gauge | none | seconds | all | age of the stalest registered heartbeat; rising age indicates worker/NATS trouble |
| `straw_runtime_state_available` | gauge | none | boolean (`0`/`1`) | HA | `1` only while Redis coordination is reachable |
| `straw_runtime_state_operations_total` | counter | none | operations | HA | shared Redis coordination operations attempted |
| `straw_runtime_state_errors_total` | counter | none | errors | HA | failed shared Redis coordination operations |
| `straw_receipts_created_total` | counter | none | receipts | receipts | durable receipts created |
| `straw_receipt_parts_uploaded_total` | counter | none | parts | receipts | parts uploaded or replaced |
| `straw_receipts_verified_total` | counter | none | receipts | receipts | receipts passing size and checksum verification |
| `straw_receipts_rejected_total` | counter | none | receipts | receipts | receipts rejected for size or checksum mismatch |
| `straw_receipt_assignments_total` | counter | none | assignments | receipts | assignment-scoped receipt references issued |
| `straw_receipts_consumed_total` | counter | none | receipts | receipts | request receipts consumed successfully |
| `straw_receipts_expired_total` | counter | none | receipts | receipts | expired receipts removed by cleanup |

Expose metrics only to your monitoring network. No telemetry database is required.

### Scrape and alert examples

Scrape the Control metrics port (`9090` by default) from the monitoring network. For HA, add one target per Control
instance; the load balancer's API listener is not the metrics endpoint:

```text
scrape_configs:
  - job_name: straw-control
    metrics_path: /metrics
    static_configs:
      - targets: ["control.example.internal:9090"]
```

The checked-in starter rules are deliberately small. Their PromQL and windows are:

| Alert | Expression | For | Suggested response |
| --- | --- | --- | --- |
| `StrawNoAvailableWorkers` | `straw_workers_available == 0` | 2m | Check worker readiness, pool eligibility, and NATS before adding capacity. |
| `StrawNATSErrors` | `rate(straw_nats_errors_total[5m]) > 0` | 5m | Inspect NATS health, credentials, reconnects, and payload limits. |
| `StrawHAStateUnavailable` | `straw_runtime_state_available == 0` | 1m | Restore Redis coordination before admitting HA traffic. |
| `StrawReceiptRejections` | `increase(straw_receipts_rejected_total[15m]) > 0` | none | Inspect declared size/checksum, part uploads, storage, and clock/credential errors. |

Copy and adapt `deploy/monitoring/prometheus-alerts.yml`; choose notification routing and objective thresholds in
your monitoring system. These rules do not replace a production alert policy.

## Logs

Control and Egress write structured JSON to stdout. Collect container stdout with the logging system already used by
your environment. Request IDs connect client errors, Control logs, and worker activity. Common fields are
`timestamp`, `level`, `msg`, and `service`; event-specific bounded fields include `request_id`, `worker_id`, `addr`,
and durations. The startup NATS `url` field is credential-redacted. Never emit bearer tokens, service credentials,
upstream or signed receipt URLs, headers, or bodies. A safe escalation bundle contains versions, profile, redacted
config shape, health/readiness, metric names/values, and request IDs only.

A representative startup event is:

```json
{"timestamp":"2026-07-14T00:00:00Z","level":"INFO","msg":"listening","service":"control","addr":"0.0.0.0:8080"}
```

Treat event-specific fields and messages as diagnostic context rather than a stable machine API; use metric names and
documented error codes for automation.

## Suggested service indicators and alerts

Track successful request ratio, p95/p99 latency, assignment availability, worker saturation, NATS errors, Redis
availability for HA, and receipt rejection/expiry. Choose objectives from your traffic and upstreams; 99.9% success
for eligible requests is an example, not a promise. Starter alerts live in
`deploy/monitoring/prometheus-alerts.yml` and are intentionally not a universal production policy.

## Scaling and shutdown

Scale Egress workers for outbound concurrency. A single Control is the simplest deployment. For Control HA, place at
least two Redis-backed Controls behind a readiness-aware load balancer. On SIGTERM, Control fails readiness and gives
active HTTP requests up to the configured maximum request timeout plus five seconds to finish before draining NATS.

## Operate executor pools

Create or update pools in the runtime snapshot before adding workers that claim them. Verify that each worker's
`pools`, executor type, tags, countries, regions, and IP types match the intended pool. A disabled pool is a safe
cutover control: it stops new assignments without cancelling requests already running. Re-enable it only after the
worker rollout and capability claims are verified. If a pool has no eligible worker, requests matching its rule return
`route_unavailable`; a full eligible fleet returns `executor_capacity_exhausted`.

Keep pool definitions and official-worker `allowed_pools` in the same deployment trust boundary. Pool membership is
not tenant authorization, and Straw does not provide cross-deployment or per-user pool permissions; run a separate
deployment when isolation is required.

## Upgrades

Read `CHANGELOG.md`, test the new version against a staging deployment, update workers before or with Control, and
keep protocol-coupled custom workers pinned until verified.

When the runtime-administration profile is enabled, include the NATS JetStream configuration bucket in backup and
recovery drills. Inspect rollout status after upgrades; an official worker reports `applied` after receiving the
current snapshot. See [Runtime administration](runtime-administration.md#back-up-and-recover).

When receipt storage is enabled, back up durable receipt records and verified bodies according to their explicit
retention, monitor rejection/expiry counters, and test cleanup plus interrupted-upload recovery. The S3 bucket or
local volume is optional application data; NATS and Redis never contain receipt bodies.

The owned local backup/restore drills are `make state-backup-smoke PROFILE=admin` and
`make state-backup-smoke PROFILE=receipts`. They stop a uniquely named disposable stack, archive its named volume,
delete/recreate the volume, restore it, restart services, and verify runtime configuration or receipt content. Never
point these commands at shared or production Compose projects. Production backup tooling must provide equivalent
application-consistent snapshots, encryption, retention, and restore verification.

Run `make ha-smoke` for the owned HA failure drill. It creates a uniquely named disposable stack, scales Egress to
two workers, verifies service through one Control loss, confirms both Controls become unready during a Redis outage
and recover afterward, then stops one worker gracefully and verifies requests continue. It removes all namespaced
containers, networks, and volumes on exit.

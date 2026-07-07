# Handoff

Task: `docs/tasks/p1/13-observability-dashboards.md`

## Changed

- Added Grafana dashboard assets under `deploy/observability/grafana/dashboards/` for Control requests, request lifecycle latency, worker health, NATS/ClickHouse transport, health/readiness, outage behavior, and routing SLOs.
- Added Grafana provisioning, Prometheus scrape/probe config, blackbox exporter config, and an optional `observability` compose profile for local deployment.
- Added `deploy/observability/dashboard_test.go` to parse the dashboard and prove required planning signals, outage/SLO panels, private-label exclusions, and compose/provisioning mounts.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Dashboards cover documented P0/P1 operational signals. | VERIFIED | `deploy/observability/grafana/dashboards/straw-operational-overview.json:53`, `deploy/observability/grafana/dashboards/straw-operational-overview.json:140`, `deploy/observability/grafana/dashboards/straw-operational-overview.json:195`, `deploy/observability/grafana/dashboards/straw-operational-overview.json:235`, `deploy/observability/grafana/dashboards/straw-operational-overview.json:275`, `deploy/observability/grafana/dashboards/straw-operational-overview.json:315` | `TestStrawOperationalDashboardCoversPlanningSignals` |
| SLO panels and outage panels are present. | VERIFIED | `deploy/observability/grafana/dashboards/straw-operational-overview.json:348`, `deploy/observability/grafana/dashboards/straw-operational-overview.json:400` | `TestStrawOperationalDashboardCoversPlanningSignals` |
| Dashboard assets are deployable with the local/prod observability stack. | VERIFIED | `docker-compose.yml:134`, `docker-compose.yml:140`, `docker-compose.yml:152`, `docker-compose.yml:155`, `docker-compose.yml:166`, `deploy/observability/grafana/provisioning/dashboards/straw.yml:11`, `deploy/observability/grafana/provisioning/datasources/prometheus.yml:4`, `deploy/observability/prometheus.yml:5`, `deploy/observability/prometheus.yml:43` | `TestGrafanaProvisioningMatchesComposeMounts` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Control `/metrics` dashboard coverage. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:53` |
| `straw_requests_total`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:53` |
| `straw_request_duration_seconds`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:140` |
| `straw_routing_duration_seconds`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:150` |
| `straw_assignment_duration_seconds`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:160` |
| `straw_active_requests`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:104` |
| `straw_worker_sessions`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:235` |
| `straw_workers_available`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:240` |
| `straw_worker_heartbeat_age_seconds`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:245` |
| `straw_nats_request_duration_seconds`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:165` |
| `straw_nats_errors_total`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:275` |
| `straw_clickhouse_write_queue_depth`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:280` |
| `straw_clickhouse_write_errors_total`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:285` |
| `straw_rate_limit_rejections_total`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:200` |
| `straw_quota_rejections_total`. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:205` |
| Controlled Prometheus label cardinality and no full URLs/worker IDs in shared dashboards. | implemented | `deploy/observability/dashboard_test.go:52`, `deploy/observability/README.md:11` |
| Egress direct metrics exposure decision. | out of scope | Resolved by `docs/planning/32-open-decisions.md` ("P1 Egress Metrics Exposure — Resolved 2026-07-06") and implemented by `docs/tasks/p1/15-egress-metrics-exposure.md`. |
| Structured JSON logs fields. | already existed | Logging shipped by `docs/tasks/p0/37-structured-json-logging.md`; this task adds metric dashboards only. |
| Routing/coordination SLO p50 < 100 ms, p99 < 500 ms. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:400` |
| Control graceful-shutdown readiness/drain behavior. | implemented | Health/readiness probe panel at `deploy/observability/grafana/dashboards/straw-operational-overview.json:315`; operational behavior already implemented by earlier P0 tasks. |
| Worker graceful-shutdown draining behavior. | implemented | Worker session/availability/heartbeat panel at `deploy/observability/grafana/dashboards/straw-operational-overview.json:235`; operational behavior already implemented by earlier P0 tasks. |
| Postgres outage behavior. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:350` |
| Redis outage behavior. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:355` |
| NATS outage behavior. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:360` |
| ClickHouse outage behavior. | implemented | `deploy/observability/grafana/dashboards/straw-operational-overview.json:365` |
| Object storage outage behavior. | out of scope | P2 BodyRef behavior; the `P2 BodyRef Response-Body Mode` decision is now resolved in `docs/planning/32-open-decisions.md` (2026-07-07) and implementation is owned by `docs/tasks/p2/05-bodyref-transport-selection.md`, `docs/tasks/p2/06-object-storage-foundation.md`, and `docs/tasks/p2/08-bodyref-response-body-flow.md`. Dashboard includes the P2 outage row for visibility at `deploy/observability/grafana/dashboards/straw-operational-overview.json:370`. |

## Verification

```sh
go test ./deploy/observability
make check
```

Result:

- `go test ./deploy/observability`: passed.
- `make check`: passed.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped (diff adds deployable observability assets; it does not touch the runtime request path).

## Reviewer Start Points

- `deploy/observability/grafana/dashboards/straw-operational-overview.json`
- `docker-compose.yml`
- `deploy/observability/dashboard_test.go`

## Remaining Work

- None.

## Blockers

- Commit/push is blocked until the pre-existing unmerged `docs/tasks/p0.md` state and unrelated staged edits are resolved or explicitly included.
  [Update 2026-07-07 sweep: stale process blocker — the current sweep started from a clean working tree; no product
  gap or owning task is required.]

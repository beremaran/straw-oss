# Handoff

Task: `docs/tasks/p1/15-egress-metrics-exposure.md`

## Changed

- `internal/config/config.go`: added Control `server.egress_metrics_enabled`.
- `cmd/control/main.go`: gates P1 Egress metric collector registration on that flag.
- `internal/control/worker_registry.go`: extends aggregate worker stats with active requests, max concurrency, and available capacity.
- `internal/control/metrics.go`: adds aggregate unlabeled Egress gauges:
  - `straw_egress_active_requests`
  - `straw_egress_max_concurrency`
  - `straw_egress_available_capacity`
- `internal/control/metrics_test.go`, `cmd/control/metrics_test.go`: cover aggregate cardinality, flag enabled/disabled behavior, and runtime-store outage fallback.
- `deploy/docker/control.json`, `docs/public/operations.md`: document/enable the Control-side flag and metrics surface only.
- `docs/tasks/p1.md`, `docs/tasks/p1/15-egress-metrics-exposure.md`: marked the task done after independent verification and `make check`.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Matches resolved decision: Control-aggregated Egress metrics only. | VERIFIED | `cmd/control/main.go:178`, `internal/control/metrics.go:255`, `internal/control/worker_registry.go:731` | `internal/control/metrics_test.go:358` |
| Explicit enablement flag. | VERIFIED | `internal/config/config.go:76`, `cmd/control/main.go:184`, `deploy/docker/control.json:8` | `cmd/control/metrics_test.go:12` |
| Metrics cannot expose unbounded tenant/request labels. | VERIFIED | `internal/control/metrics.go:256` registers plain `GaugeFunc`s with no labels. | `internal/control/metrics_test.go:379` |
| No worker-local `/metrics` endpoint. | VERIFIED | `cmd/egress/main.go:170`, `docs/public/operations.md:57` | Code/docs inspection by verifier. |
| No worker metrics port shipped. | VERIFIED | `internal/config/config.go:158`, `deploy/docker/egress.json:8`, `deploy/docker/README.md:19` | Code/docs inspection by verifier. |
| Outage behavior when Control cannot aggregate. | VERIFIED | `internal/control/worker_registry.go:705`, `internal/control/worker_registry.go:731`, `docs/public/operations.md:58` | `internal/control/metrics_test.go:384` |
| Deployment docs expose only Control-side metrics surfaces and flag. | VERIFIED | `docs/public/operations.md:31`, `docs/public/operations.md:56`, `deploy/docker/README.md:18` | Docs/code inspection by verifier. |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Control exposes Prometheus metrics at `/metrics` (`docs/planning/23`). | already existed | `cmd/control/main.go:395`, `docs/public/operations.md:31` |
| P1 uses Control-aggregated Egress metrics only (`docs/planning/23`, `docs/planning/32`). | implemented | `cmd/control/main.go:184`, `internal/control/metrics.go:255` |
| Egress reports bounded telemetry to Control over the service boundary (`docs/planning/23`). | implemented | Existing heartbeat state is the service boundary source: `internal/control/worker_registry.go:701`, `internal/control/worker_registry.go:731` |
| Control exposes resulting Prometheus series on its metrics surface (`docs/planning/23`). | implemented | `cmd/control/main.go:184`, `internal/control/metrics.go:255`, `docs/public/operations.md:56` |
| Direct worker Prometheus scraping is not a shipped P1 mode (`docs/planning/23`, `docs/planning/32`). | implemented | No Egress metrics endpoint added; `cmd/egress/main.go:170`, `docs/public/operations.md:57` |
| Explicit enablement flag (`docs/planning/23`, `docs/planning/32`). | implemented | `internal/config/config.go:76`, `cmd/control/main.go:184`, `deploy/docker/control.json:8` |
| Metric cardinality must be controlled; no high-cardinality URL labels (`docs/planning/23`). | implemented | Unlabeled aggregate gauges: `internal/control/metrics.go:252`, `internal/control/metrics_test.go:379` |
| Control port 9090 is the metrics surface (`docs/planning/28`). | already existed | `deploy/docker/README.md:18`, `docs/public/operations.md:31` |
| Do not map unused worker metrics ports (`docs/planning/28`, `docs/planning/32`). | implemented | `deploy/docker/egress.json:8`, `deploy/docker/README.md:19` |

## Verification

```sh
go test ./internal/control ./cmd/control ./internal/config
make check
```

Result:

- Focused tests: passed.
- `make check`: passed (`go test ./...`; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` with 0 issues).
- Postgres-backed tests: not exercised; diff does not touch Postgres files or migrations.
- Live compose verification: skipped; diff changes Control metrics exposure/aggregation, not the runtime request path.

Completion audit notes:

- Diff grep for `InMemory`, `stub`, `fake`, `synthetic`, and `TODO` found only existing repo test doubles plus `failingRuntimeStore` in `internal/control/metrics_test.go:412`, a focused outage test double. No production fake, stub, or unowned deferral was introduced.
- Diff grep for `no owning task`, `no owner`, `future work`, and `if needed later` found no new deferrals.

## Reviewer Start Points

- `cmd/control/main.go:178`
- `internal/control/metrics.go:252`
- `internal/control/worker_registry.go:701`
- `internal/control/metrics_test.go:358`
- `cmd/control/metrics_test.go:12`
- `docs/public/operations.md:55`

## Remaining Work

- None.

## Blockers

- None.

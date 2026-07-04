# Handoff

Task: `docs/tasks/p0/28-control-observability-metrics.md`

## Changed

- `internal/control/metrics.go` (new): `Metrics` struct wrapping the P0 Prometheus series
  (`docs/planning/23-observability.md`). `NewMetrics(reg prometheus.Registerer) *Metrics` builds and registers the
  push-based series: `straw_requests_total{tenant_id,error_code}`, `straw_request_duration_seconds{tenant_id}`,
  `straw_routing_duration_seconds`, `straw_assignment_duration_seconds`, `straw_active_requests`,
  `straw_nats_request_duration_seconds`, `straw_nats_errors_total{error_code}`,
  `straw_clickhouse_write_errors_total`, `straw_rate_limit_rejections_total{tenant_id}`,
  `straw_quota_rejections_total{tenant_id}`. Every `Observe*`/`Inc*`/`Dec*` method is nil-receiver-safe so callers
  never need a nil check and existing tests that build dispatcher/admission/writer components without a `Metrics`
  keep working unchanged.
  `RegisterWorkerCollector(reg, source)` and `RegisterClickHouseQueueDepth(reg, source)` register three more series
  as `GaugeFunc`s computed at scrape time rather than pushed: `straw_worker_sessions`, `straw_workers_available`,
  `straw_worker_heartbeat_age_seconds` (from `*WorkerRegistry`), and `straw_clickhouse_write_queue_depth` (from
  `*RequestMetadataWriter`). Pull-based was the right shape here: these describe current registry/queue state, not
  discrete events, and `worker_id` is not an allowed label (only `tenant_id`, `target_host`, `route_id`,
  `error_code` per `docs/planning/23`), so per-worker detail is aggregated into registry-wide counts/max instead.
- `internal/control/worker_registry.go`: added `WorkerRegistryStats` and `(*WorkerRegistry).Stats()`, computing
  session count, available (ready/degraded) count, and the oldest current heartbeat's age under the existing
  registry mutex. No behavior change to any existing method.
- `internal/control/request_metadata.go`: added `(*RequestMetadataWriter).SetMetrics` and `.QueueDepth()`
  (mutex-guarded `len(queue)`), and `Flush` now calls `metrics.IncClickHouseWriteError()` on a failed batch write.
- `internal/control/ratelimit.go`: added `(*RateLimitAdmission).SetMetrics`; `Check` now calls
  `metrics.IncRateLimitRejection(tenant_id)` when the worst dimension decision denies, which also covers the
  Redis-fail-closed path since that surfaces through the same `worst` decision.
- `internal/control/quota_admission.go`: added `(*QuotaAdmission).SetMetrics`; both `CheckAdmission`'s script-driven
  denial and `failureDecision`'s Redis-fail-closed path call `metrics.IncQuotaRejection(tenant_id)`. Also introduced
  `quotaFailPolicyClosed` constant (was a bare `"closed"` literal; needed once a second call site was added to keep
  `goconst` at zero issues).
- `internal/control/dispatcher.go`: `RequestDispatcherOptions.Metrics *Metrics` (optional). `Dispatch` now wraps the
  renamed `dispatch` (former body) to record `IncActiveRequests`/`DecActiveRequests` and
  `ObserveRequest(tenant_id, error_code, total_duration)` around every attempt, success or failure.
  `ObserveRouting` is recorded right after route evaluation (before the `RouteNoMatch`/`RouteUnavailable` early
  return, so a failed route still contributes a sample). `executeAttempt` now delegates to
  `executeAttemptUnmeasured` and calls `ObserveAssignment` once on the returned `assignmentMs` — this (rather than a
  named-return `defer`) was needed to satisfy the `nonamedreturns` linter. `requestAssign` records
  `ObserveNATSRequest` around the NATS `Request` round trip and `IncNATSError(error_code)` on timeout, transport
  failure, or protocol error.
- `cmd/control/health.go`: `newMetricsMux` takes a `*prometheus.Registry` and serves it at `/metrics` via
  `promhttp.HandlerFor`, alongside the existing `/healthz`/`/readyz`.
- `cmd/control/main.go`: new `wireMetrics(workerRegistry, metadataWriter) (*prometheus.Registry, *control.Metrics)`
  called from `runControl`, building the registry with `control.NewMetrics`, registering the worker collector
  unconditionally and the ClickHouse queue-depth collector only when a `metadataWriter` exists (no ClickHouse
  endpoint configured means the gauge is simply absent, matching the existing "telemetry disabled" degraded mode).
  `buildControlMux` now takes `metrics *control.Metrics`, threads it into a freshly-constructed
  `RateLimitAdmission`/`QuotaAdmission` pair via `SetMetrics` and into `RequestDispatcherOptions.Metrics`, and
  `wireClickHouse`'s writer gets `SetMetrics` in `wireMetrics`. `serveControl`/`serveMetricsHTTP` take the registry
  and pass it to `newMetricsMux`. Note: `AdminHandlers.RateLimitAdmission`/`.QuotaAdmission` are a separate,
  currently-unused pair of instances (confirmed via grep — nothing calls `.Check`/`.CheckAdmission` on them); only
  the dispatcher's instances sit on the live admission path, so only those got `SetMetrics`.
- `go.mod`/`go.sum`: added `github.com/prometheus/client_golang` (direct) and its transitive deps
  (`client_model`, `common`, `procfs`, `beorn7/perks`, etc.) via `go get` + `go mod tidy`.
- Tests: `internal/control/metrics_test.go` (new) covers series registration, counter/histogram movement on every
  `Metrics` method, the nil-receiver-safety contract, `RateLimitAdmission`/`QuotaAdmission` rejection recording
  against a deliberately-unreachable Redis (fail-closed), `RequestMetadataWriter` write-error/queue-depth recording
  against a failing sink, and `RegisterWorkerCollector` against a real `WorkerRegistry` with a fake clock
  (sessions/available/heartbeat-age before and after a heartbeat, and heartbeat age advancing with the clock).
  `cmd/control/health_test.go` updated for the new `newMetricsMux` signature and a new `TestMetricsServesRegisteredSeries`
  proving `GET /metrics` is 200 and contains a recorded series end-to-end through the real mux.

## Verification

```sh
go test ./internal/control/... ./cmd/... -v -run Metric
make check
```

Result: all new/updated tests pass; `make check` (gofmt + `go test ./...` + `golangci-lint run
--max-issues-per-linter 0 --max-same-issues 0`) is clean (0 lint issues).

Additionally ran the real stack live (`docker compose up -d --build`) and confirmed `GET http://localhost:9090/metrics`
returns `200` with the expected `# HELP`/`# TYPE` lines for the always-present series (`straw_active_requests`,
`straw_assignment_duration_seconds`, `straw_clickhouse_write_errors_total`, `straw_clickhouse_write_queue_depth`,
`straw_nats_request_duration_seconds`, `straw_routing_duration_seconds`, plus the worker gauges). The
`tenant_id`/`error_code`-vectored counters (`straw_requests_total`, `straw_rate_limit_rejections_total`,
`straw_quota_rejections_total`, `straw_nats_errors_total`) correctly carry no series in that fresh scrape — this is
standard `CounterVec`/`HistogramVec` behavior (no series until a label combination is first observed) and is
exercised at the unit level in `metrics_test.go` instead. I did not push a real REST request through the live stack
to observe these move: the compose egress worker fails to register against a freshly-provisioned Postgres (a
pre-existing, documented gap — see `deploy/docker/README.md` "Worker provisioning" and the compose file's own
comment on the `egress` service) and provisioning a tenant API key + worker credential by hand was out of scope for
this task. Torn the stack back down (`docker compose down`) after the check.

## Verified against Acceptance Criteria

- "`GET /metrics` on the metrics port returns 200 with the P0 metric series named above." — true for series with no
  dynamic labels always; true for `tenant_id`/`error_code`-vectored series once at least one event has occurred
  (verified in `metrics_test.go` and in the live check above). This is the correct Prometheus semantics for a
  label built from an unbounded value (tenant_id) that Control cannot enumerate at startup.
- "Counters and histograms move when the corresponding events occur (verified in tests)." — done, see
  `metrics_test.go`.
- "No metric carries a high-cardinality label such as a full URL." — done; the only dynamic labels used anywhere
  are `tenant_id` and `error_code`, both from the allowed set in `docs/planning/23`. `target_host`/`route_id` are
  not used by any P0 metric in the given list, so they were not added speculatively.

## Reviewer Start Points

- `internal/control/metrics.go`
- `internal/control/dispatcher.go` (`Dispatch`/`dispatch` split, `executeAttempt`/`executeAttemptUnmeasured` split,
  `requestAssign`)
- `cmd/control/main.go` (`wireMetrics`, `buildControlMux`)
- `cmd/control/health.go` (`newMetricsMux`)
- `internal/control/metrics_test.go`

## Remaining Work

- None owned by this task. The optional direct worker Prometheus scrape (`docs/planning/31` P1 item 5) and any
  telemetry read APIs/dashboards remain out of scope per the task's own "Out of Scope" section and are not touched
  here.
- Pre-existing, unrelated to this change: the compose `egress` service cannot complete worker registration against
  a fresh stack without manual worker-credential provisioning (documented in `deploy/docker/README.md`). This
  limited live verification of the NATS-path metrics (`straw_nats_request_duration_seconds`,
  `straw_nats_errors_total`, `straw_assignment_duration_seconds`, `straw_worker_sessions`,
  `straw_workers_available`, `straw_worker_heartbeat_age_seconds` moving from real traffic) to the unit-test level;
  no P0 task currently owns fixing that provisioning gap.

## Blockers

- None.

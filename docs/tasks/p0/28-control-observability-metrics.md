# 28 - Control Observability Metrics

Status: done

## Objective

Expose the P0 Prometheus metrics surface on Control's metrics port (`/metrics`) with the metric set defined in
`docs/planning/23-observability.md`.

## Context (gap being closed)

The 2026-07-04 review found the metrics port serves only `/healthz` and `/readyz`; `/metrics` returns 404 and none
of the `docs/planning/23` P0 metrics are implemented. No task owned this surface. Note the P0/P1 split: **Control
`/metrics` is P0** per `docs/planning/23`; the *optional direct worker Prometheus scrape* in
`docs/planning/31` P1 item 5 is a separate, deferred concern and is out of scope here.

## Required Planning Docs

- `docs/planning/23-observability.md` (P0 metric names, label-cardinality rules)
- `docs/planning/28-deployment.md` (metrics port)
- `docs/planning/29-operational-behavior.md` (what each signal should reflect)
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 24 completed (request pipeline to instrument).
- Task 25 completed (`cmd/control/health.go` metrics-port server already exists).

## Out of Scope

- Do not implement direct worker Prometheus scraping (owned by `docs/tasks/p1/15-egress-metrics-exposure.md`).
- Do not implement telemetry read APIs, dashboards, or exemplars beyond what the stdlib client offers.
- Do not add high-cardinality labels (no raw URLs; see `docs/planning/23`).

## Expected Files

- Modify: `cmd/control/health.go` (register the Prometheus handler on the metrics mux alongside `/healthz`/`/readyz`).
- Create: `internal/control/metrics.go` (metric registry, definitions, and the increment/observe helpers).
- Modify: `internal/control/dispatcher.go`, `internal/control/ratelimit.go`,
  `internal/control/quota_admission.go`, `internal/control/worker_registry.go`,
  `internal/control/request_metadata.go` (call the instrumentation hooks).
- Modify: `cmd/control/main.go` (construct the metrics registry and thread it into the components).
- Modify: `go.mod`/`go.sum` (add `github.com/prometheus/client_golang`; the planning doc mandates Prometheus
  exposition format, which the stdlib does not provide).
- Test: `internal/control/metrics_test.go`.

## Steps

- [x] Read all required planning docs.
- [x] Add the `prometheus/client_golang` dependency and a metrics registry constructed at startup.
- [x] Define the P0 metrics from `docs/planning/23`: `straw_requests_total`, `straw_request_duration_seconds`,
      `straw_routing_duration_seconds`, `straw_assignment_duration_seconds`, `straw_active_requests`,
      `straw_worker_sessions`, `straw_workers_available`, `straw_worker_heartbeat_age_seconds`,
      `straw_nats_request_duration_seconds`, `straw_nats_errors_total`, `straw_clickhouse_write_queue_depth`,
      `straw_clickhouse_write_errors_total`, `straw_rate_limit_rejections_total`, `straw_quota_rejections_total`.
- [x] Instrument the dispatch pipeline (request totals/durations, routing/assignment durations, active requests,
      NATS request duration/errors), the rate limiter and quota admission (rejections), the worker registry (session
      counts, availability, heartbeat age), and the ClickHouse writer (queue depth, write errors).
- [x] Bound label cardinality to `tenant_id`, `target_host`, `route_id`, `error_code` per `docs/planning/23`; never
      label full URLs.
- [x] Serve the Prometheus handler at `/metrics` on the metrics port.
- [x] Add tests that scrape the registry and assert the named series exist and move under simulated load.
- [x] Run the focused tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- `GET /metrics` on the metrics port returns 200 with the P0 metric series named above.
- Counters and histograms move when the corresponding events occur (verified in tests).
- No metric carries a high-cardinality label such as a full URL.

## Handoff Notes

- Document each metric, its labels, and where it is incremented/observed.
- Note the P0-vs-P1 boundary (Control `/metrics` P0; worker scrape P1).

## Stop Conditions

- Stop before adding worker-scrape or telemetry read APIs.
- Stop if a required metric has no clear P0 signal source and would be fabricated.
- Stop if a deferral would have no owning task file.

# Handoff

Task: `docs/tasks/p0/38-egress-health-endpoints.md`

## Changed

- `internal/config/config.go`: added `EgressConfig.HealthPort` (`health_port`), defaulting to `8090` and validated
  1-65535 (same shape as Control's `api_port`/`metrics_port`).
- `cmd/egress/health.go` (new): `newHealthMux(ready *atomic.Bool)` serving `/healthz` (always 200) and `/readyz`
  (200 only when `ready` is true, else 503), mirroring `cmd/control/health.go`.
- `cmd/egress/main.go`: added `-healthcheck` flag (`loadEgressConfig`/`runHealthcheck`, mirroring Control's pattern
  for distroless-image container healthchecks) and `serveHealthHTTP`, which starts/stops the health mux on
  `cfg.HealthPort` around the run loop in `runWorker`. A `*atomic.Bool` is created per worker process and passed
  into `egress.Run`.
- `internal/egress/runtime.go`: `Run` now takes a `*atomic.Bool` ready parameter (nil-safe via the new `setReady`
  helper). Readiness state transitions:
  - `false` at the start of `Run`,
  - `true` immediately after `Register` succeeds (RegisterAck OK),
  - `false` again if `NewWorker`/the initial heartbeat fails, or once `ctx.Done()` fires and the drain/final
    heartbeat sequence begins.
  Extracted the post-setup ticker loop into `runHeartbeatLoop` to keep `Run` under the `funlen`/`cyclop` limits.
- `internal/control/worker_nats_test.go`: `TestWorkerRunLoopAppearsInAdminWorkersAndDrainsOnCancel` now passes a
  `ready` flag into `egress.Run` and asserts it is `true` once Control reports the worker `Ready` and `false` once
  the run loop returns after `cancel()`.
- `cmd/egress/health_test.go` (new): exercises `/healthz` regardless of readiness and `/readyz` before/after/during
  the ready flag flipping.
- `internal/config/config_test.go`: added an out-of-range `health_port` case and asserted the default applies when
  unset.
- `deploy/docker/egress.json`: sets `health_port: 8090`.
- `docker-compose.yml`: added an `egress` service healthcheck running `/egress -config ... -healthcheck` (same
  pattern as `control`'s `-healthcheck` probe against its own `/readyz`), no host port published (nothing outside
  the container needs to reach it, matching docs/planning/28 "do not map unused ports").
- `deploy/docker/README.md`: documented the egress health port in the ports table.

## Verification

```sh
go test ./cmd/... ./internal/egress ./internal/config ./internal/control/...
make check
```

Result: all packages pass; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reports 0 issues.

## Reviewer Start Points

- `cmd/egress/health.go`
- `cmd/egress/main.go` (`loadEgressConfig`, `runHealthcheck`, `serveHealthHTTP`, `runWorker`)
- `internal/egress/runtime.go` (`Run`, `runHeartbeatLoop`, `setReady`)
- `internal/control/worker_nats_test.go` (`TestWorkerRunLoopAppearsInAdminWorkersAndDrainsOnCancel`)

## Remaining Work

- None for this task's scope. Worker `/metrics` scrape stays P1 (`docs/tasks/p1/15-egress-metrics-exposure.md`).
  In docker compose, `/readyz` only turns green once live registration succeeds, which needs task 35's signing
  path (already completed) plus a real Control instance up — this is a runtime/environment condition, not a gap in
  this task, and matches the task file's own prerequisite note.

## Blockers

- None.

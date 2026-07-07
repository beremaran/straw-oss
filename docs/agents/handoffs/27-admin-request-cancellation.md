# Handoff

Task: `docs/tasks/p0/27-admin-request-cancellation.md`

## Changed

- `internal/control/inflight.go` (new): `InFlightRegistry` mapping `request_id -> {tenant_id, cancel func()}`.
  `Register`/`Deregister` are called by the dispatcher; `Cancel` authorizes via `AuthorizeAdminCancel`
  (`lifecycle.go`, already existed) and, if authorized, invokes the stored cancel func. Nil-safe (all methods no-op
  or return `ErrRequestNotFound` on a nil receiver) so tests and non-P0 callers can leave it unwired.
- `internal/control/dispatcher.go`: `Dispatch` now wraps its incoming `ctx` in `context.WithCancel`, registers
  `(request_id, tenant_id, cancel)` with `d.opts.InFlight` before doing any work, and deregisters via `defer` on
  every return path (success, pipeline error, or panic-free early return). No other dispatcher logic changed —
  the existing `ctx.Done()` branch in `readResponse` (client-disconnect/deadline cancellation) is the same code path
  an admin cancel now drives; that branch already published `CancelFrame` on the request's `c2e` subject and
  returned `PipelineError{Code: Cancelled}` before this task, so admin cancel reuses tested behavior rather than
  adding a new one. Added `RequestDispatcherOptions.InFlight *InFlightRegistry` (optional field).
- `internal/control/request_admin_handlers.go` (new): `CancelRequest` handles
  `POST /api/v1/admin/requests/{request_id}/cancel`. Authenticates, requires role in
  `{system_admin, tenant_admin, operator}` (docs/planning/26), then calls `InFlightRegistry.Cancel`. Maps
  `ErrInsufficientPermissions -> 403 insufficient_permissions`, `ErrRequestNotFound -> 400 invalid_request` (platform
  caller cancelling an unknown request_id only — see judgment call below), nil `InFlight` -> `500
  control_internal_error`. On success returns `200 {"request_id", "status":"cancelling"}` and records an audit row.
- `internal/control/admin_handlers.go`: added `InFlight *InFlightRegistry` field to `AdminHandlers`.
- `cmd/control/main.go`: `buildControlMux` constructs one `control.NewInFlightRegistry()` per Control process and
  wires it into both `RequestDispatcherOptions.InFlight` and `AdminHandlers.InFlight` (same instance, so the
  dispatcher's registrations are visible to the admin handler). `buildAdminHandlers` takes the registry as a new
  parameter. Registered `POST /api/v1/admin/requests/{request_id}/cancel` in `serveAdminRoutes`.
- Tests: `internal/control/inflight_test.go` (registry unit tests), `internal/control/request_admin_handlers_test.go`
  (handler tests against the live route dispatch, not just the auth predicate), and two new tests appended to
  `internal/control/dispatcher_test.go` (`TestDispatcherAdminCancelEndToEnd`,
  `TestDispatcherAdminCancelForeignTenantRejected`) extending the existing live NATS/egress-worker dispatch harness.
- `docs/agents/testing-matrix-audit.md`: updated the Cancellation and Worker admin rows to cite the new endpoint
  and registry tests instead of only `AuthorizeAdminCancel`.

## Judgment call: unknown request_id for a platform caller

The task spec and `docs/planning/26` define behavior for a **tenant-scoped** caller on a foreign or unknown
request_id (`insufficient_permissions`, no disclosure) but not for a **platform** `system_admin` cancelling an
unknown request_id — there is no confidentiality concern for platform scope, but the canonical error registry
(`docs/planning/14`) has no generic "not found" code. I mapped this case to `invalid_request` (400) with
`details.reason = "unknown request_id"` — the closest canonical fit ("malformed request or missing business
fields"). This is a minor, low-risk interpretation; flagging it in case a future task wants a dedicated code.

## Verification

```sh
go test ./internal/control/... -run 'InFlight|CancelRequest|DispatcherCancellation|DispatcherAdminCancel' -v
make check
```

Result: all new tests pass; `make check` (gofmt + `go test ./...` + `golangci-lint run --max-issues-per-linter 0
--max-same-issues 0`) is clean (0 lint issues).

## Reviewer Start Points

- `internal/control/inflight.go`
- `internal/control/request_admin_handlers.go`
- `internal/control/dispatcher.go` (the `Register`/`defer Deregister` addition in `Dispatch`)
- `cmd/control/main.go` (`buildControlMux`, `buildAdminHandlers`, `serveAdminRoutes`)
- `internal/control/dispatcher_test.go` (`TestDispatcherAdminCancelEndToEnd`,
  `TestDispatcherAdminCancelForeignTenantRejected`)

## Remaining Work

- None owned by this task. The registry is intentionally in-process/runtime-only for single-Control P0 (task's
  "Out of Scope" section, consistent with the task 13 worker-runtime-state deferral) — it does not survive a
  Control restart and is not shared across Control instances. Multi-Control coordination and durable cancellation
  state are out of scope for P0 and have no owning P0 task; if needed later they'd be a new P1/P2 task.
  2026-07-06 update: this gap became owned by `docs/tasks/p1/23-multi-control-durable-cancellation-state.md`
  on the P1 board while waiting for a committed multi-Control-replica decision.
  2026-07-07 update: the multi-Control-replica decision is committed (`docs/planning/32` "Multiple Concurrent
  Control Replicas — Resolved 2026-07-07") and `docs/tasks/p1/23` is **done**. Cross-instance admin cancel now
  reaches the owning Control replica via the Redis runtime-state tier (opt-in `server.multi_control_enabled`); the
  single-Control in-process fast path here is unchanged. This gap is closed.

## Blockers

- None.

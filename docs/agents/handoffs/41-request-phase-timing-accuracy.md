# Handoff

Task: `docs/tasks/p0/41-request-phase-timing-accuracy.md`

## Changed

- `internal/egress/executor.go` — `Executor.Execute` gains a `send func(*strawpb.StreamFrame)` parameter and now
  delivers the `OutboundStartFrame` through it before DNS/connect (docs/planning/09 step 19); with a nil `send` the
  frame stays in the returned batch (unit tests use this). `emitOrBatch` helper keeps `Execute` under the cyclop
  limit.
- `internal/egress/loop.go` — `runRequest` passes a `send` closure that publishes OutboundStart to the e2c subject
  immediately, instead of batching it with the terminal frames after the upstream call finishes.
- `internal/control/dispatcher_test.go` — new `TestDispatcherEgressPhaseTiming`: full Control→NATS→egress→upstream
  round trip against a 100ms-delayed upstream, asserting `EgressMs >= 50` and
  `RoutingMs + AssignmentMs + EgressMs <= TotalMs`. Written first; failed with `EgressMs = 0` before the fix.
- `internal/egress/executor_test.go` — new `TestExecutorSendsOutboundStartBeforeConnect` (OutboundStart delivered via
  `send` before the upstream is contacted, and excluded from the returned batch); existing `Execute` call sites pass
  `nil`; `unitTestHost` const for goconst.
- `internal/egress/loop_test.go` — drain test updated: OutboundStart now legitimately arrives while the upstream
  handler is still blocked, so the "nothing before release" assertion became "no frame other than OutboundStart
  before release". Also fixed a pre-existing hang-on-failure trap: the blocked upstream handler is now released via
  `t.Cleanup` so `server.Close` cannot deadlock when an assertion fails.
- `docs/tasks/p0.md`, `docs/tasks/p0/41-request-phase-timing-accuracy.md` — status updates.

## Root Cause

The task file pointed at Control's `dispatcher.go`, but Control was correct: it stamps `egressStarted` when the
OutboundStart frame *arrives* and computes `egress_ms` on the End frame. The bug was in the egress worker:
`Executor.Execute` accumulated every frame — OutboundStart included — into one slice, and `Worker.runRequest`
published the whole slice only after the upstream HTTP call completed. Control therefore received OutboundStart and
End microseconds apart, so `egress_ms` measured NATS delivery jitter, not the upstream fetch. This also violated
docs/planning/09 step 19 ("Executor sends OutboundStartFrame before DNS/connect").

Why the in-process tests missed it: `TestDispatcherControlNATSEgressRoundTrip` asserted status/body/headers but
never the timing fields, and no test used a delayed upstream, so a ~0ms egress phase looked indistinguishable from
a correct one.

`routing_ms = 0` is legitimate: it wraps only the in-memory `d.route` evaluation over a cached snapshot
(`dispatcher.go`), which is genuinely sub-millisecond. Verified by the phase-sum assertion in the new test and the
live rows below.

## Verification

```sh
go test ./internal/control/ -run Dispatcher   # includes TestDispatcherEgressPhaseTiming
go test ./internal/egress/
make check
```

Result: all pass; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reports 0 issues.

Live compose round-trip (fresh volumes, user-approved; `deploy/docker/README.md` procedure — bootstrap admin key,
mint dev-tenant requester key, `POST /api/v1/requests` to `https://example.com/` twice):

| request | routing_ms | assignment_ms | egress_ms | total_ms |
|---------|-----------|---------------|-----------|----------|
| req_1783223446840555069 | 0 | 8 | 124 | 140 |
| req_1783223504346174893 | 0 | 7 | 95 | 107 |

Phase sums (132/140, 102/107) are consistent with `total_ms`; the previous live rows recorded `egress_ms = 0` on
comparable ~150-250ms dispatches. No schema or protobuf changes.

## Reviewer Start Points

- `internal/egress/executor.go` (`Execute` signature, `emitOrBatch`)
- `internal/egress/loop.go` (`runRequest` send closure)
- `internal/control/dispatcher_test.go` (`TestDispatcherEgressPhaseTiming`)

## Remaining Work

- None. Nothing in this task is faked, stubbed, or deferred: the fix is in the live egress publish path, verified
  end-to-end on the compose stack.

## Blockers

- None. (One operational note: the compose Postgres volume held a system_admin key from the 2026-07-05 session, so
  the bootstrap env key could not seed; the user chose `docker compose down -v` + rebuild for the live check.)

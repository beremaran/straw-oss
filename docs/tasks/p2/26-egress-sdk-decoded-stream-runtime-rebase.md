# 26 - Egress SDK Decoded Stream Runtime Rebase

Status: not started

## Objective

Move decoded HTTP assignment handling from `internal/egress` into `sdk/egress`: exact-session assignment subscription,
subscriber flush before `AssignAck`, `RequestStart` and inline body reads, stream sequencing, cancellation, response
credit, executor error frames, and e2c publish behavior must run through the public SDK.

## Context (gap being closed)

The original task 22 was split on 2026-07-07 because it mixed the whole worker runtime in one oversized slice. After
task 22, `sdk/egress` owns registration and heartbeat, but `internal/egress/loop.go` still owns
`NewWorker`, `Serve`, `handleAssign`, `prepareRequestStream`, decoded `runRequest`, `waitForResult`, response-credit
gating, and e2c publishing. This task owns that decoded stream protocol move. Raw tunnel and BodyRef-specific hooks are
separate in task 27.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations)
- `docs/planning/12-nats-protocol.md` (assignment ordering, stream sequencing, credit, error protocol)
- `docs/planning/16-egress-execution.md` (official outbound execution stays in the worker)

## Prerequisites

- Task 22 completed (SDK owns registration/heartbeat/session runtime).

## Out of Scope

- Do not move raw CONNECT tunnel handling; task 27 owns it.
- Do not move BodyRef request-body download hooks; task 27 owns it.
- Do not add final SDK conformance/live verification; task 24 owns that after tasks 22, 23, 26, 27, and 28.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

## Expected Files

- Modify: `sdk/egress/` — add decoded assignment worker runtime and stream publish/read helpers.
- Modify: `internal/egress/loop.go` — remove or delegate decoded stream protocol code that now lives in the SDK.
- Test: `sdk/egress` decoded runtime tests for assignment accept/reject, subscriber flush before `AssignAck`, stream
  sequencing, cancellation, response credit, and executor error frames.

## Steps

- [ ] Read all required planning docs.
- [ ] Move exact-session assignment subscription and `AssignAck` handling into `sdk/egress`.
- [ ] Move decoded `RequestStart`/inline body read, cancellation, response credit, and e2c publish behavior into
      `sdk/egress`.
- [ ] Keep official outbound HTTP execution in `internal/egress.Executor`.
- [ ] Add/move the decoded runtime tests listed in Expected Files.
- [ ] Verify `sdk/egress` imports no `internal/*` packages.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `internal/egress` no longer owns decoded HTTP assignment subscription, decoded stream framing, cancellation, response
  credit, or executor error-frame protocol except as delegated compatibility wrappers.
- `sdk/egress` runtime tests prove decoded assignment accept/reject, subscriber flush before `AssignAck`, sequence
  handling, cancellation, response credit, and executor error frames.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- Existing official executor tests still pass.

## Handoff Notes

- Record what decoded runtime moved to `sdk/egress` and any temporary compatibility wrappers left behind.
- State that task 27 still owns raw tunnel and BodyRef runtime movement.

## Stop Conditions

- Stop if `sdk/egress` would need to import `internal/*`.
- Stop if preserving wire behavior requires protocol changes.
- Stop if a deferral would have no owning task file.

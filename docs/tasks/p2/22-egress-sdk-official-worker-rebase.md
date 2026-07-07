# 22 - Egress SDK Official Worker Rebase

Status: not started

## Objective

Move the official worker's live protocol runtime onto `sdk/egress`: registration, heartbeat loop, exact-session
assignment handling, request stream framing, cancellation, error protocol, and response credit behavior must run
through the public SDK while `internal/egress` keeps only the official outbound HTTP execution engine.

## Context (gap being closed)

Task 12 creates the public SDK foundation but intentionally stops before moving the live worker loop. Current code still
wires the official worker directly through `internal/egress.Run(ctx, natsConn, id, caps, executor, heartbeatInterval,
ready)` in `cmd/egress/main.go`, and `internal/egress.NewWorker` takes the concrete `*Executor` rather than a public
SDK interface. Until this task is complete, the SDK is a foundation only, not the runtime used by the official worker.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (official worker rebased onto the SDK)
- `docs/planning/12-nats-protocol.md` (assignment ordering, stream sequencing, credit, error protocol)
- `docs/planning/16-egress-execution.md` (official outbound execution stays in the worker)
- `docs/planning/32-open-decisions.md` (superseded `P2 Provider Adapter Baseline` acceptance tests)

## Prerequisites

- Task 12 completed (`sdk/egress` foundation and `Executor` interface exist).

## Out of Scope

- Do not rename `DESTINATION_RESOLUTION_PROVIDER_ADAPTER`; task 23 owns that.
- Do not add the standalone custom Egress example; task 13 owns that.
- Do not add new execution behavior; the official outbound HTTP engine remains in `internal/egress`.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

## Expected Files

- Modify: `sdk/egress/` — add or move the live worker runtime needed for registration, heartbeat, assignment,
  stream framing, cancellation, and error protocol.
- Modify: `internal/egress` — leave the official outbound executor implementation and any official-worker-only
  helpers; remove or delegate protocol runtime code that now lives in `sdk/egress`.
- Modify: `cmd/egress/main.go` — construct and run the `sdk/egress` worker with the internal executor.
- Test: moved/adapted loop, registration, runtime, and assignment tests under `sdk/egress`; `cmd/egress` wiring test.

## Steps

- [ ] Read all required planning docs.
- [ ] Move the live worker runtime from `internal/egress` to `sdk/egress` without changing NATS wire behavior.
- [ ] Make `internal/egress.Executor` implement `sdk/egress.Executor`.
- [ ] Rewire `cmd/egress` so the built `cmd/egress` binary constructs and runs the `sdk/egress` worker.
- [ ] Move/adapt existing protocol tests so SDK runtime behavior is covered under `sdk/egress`.
- [ ] Add a `cmd/egress` wiring test proving the binary path uses the SDK runtime.
- [ ] Verify `sdk/egress` imports no `internal/*` packages.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `cmd/egress/main.go` constructs the worker through `sdk/egress`, not `internal/egress.Run` or
  `internal/egress.NewWorker`.
- `internal/egress` no longer owns registration, heartbeat loop, exact-session assignment subscription, or request
  stream protocol machinery except as delegated compatibility wrappers if required by existing tests.
- `sdk/egress` runtime tests cover assignment accept/reject, subscriber flush before `AssignAck`, stream sequencing,
  cancellation, response credit, and executor error frames.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- Existing official executor tests still pass, proving outbound execution behavior stayed in `internal/egress` and did
  not change.

## Handoff Notes

- Record exactly what moved to `sdk/egress` and what stayed in `internal/egress`.
- Record the `cmd/egress` wiring evidence.
- State that task 24 still owns independent SDK conformance plus live compose verification.

## Stop Conditions

- Stop if the rebase cannot preserve wire behavior without protocol changes.
- Stop if `sdk/egress` would need to import `internal/*`.
- Stop if a deferral would have no owning task file.

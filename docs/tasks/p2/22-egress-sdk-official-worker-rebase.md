# 22 - Egress SDK Official Worker Session Runtime Rebase

Status: done

## Objective

Move the official worker's session-level runtime onto `sdk/egress`: registration request/reply, bounded registration
retry, heartbeat request/reply, session loss re-registration, ready/draining state, and `cmd/egress` construction must
run through the public SDK. The exact-session assignment loop remains temporarily delegated to `internal/egress` and is
owned by follow-on tasks 26-28.

## Context (gap being closed)

Task 12 creates the public SDK foundation but intentionally stops before moving the live worker loop. Current code still
wires the official worker directly through `internal/egress.Run(ctx, natsConn, id, caps, executor, heartbeatInterval,
ready)` in `cmd/egress/main.go`; `internal/egress/runtime.go` owns registration, heartbeat, retry, session loss, and
ready/draining behavior; and `internal/egress.NewWorker` still owns the assignment stream loop. The original task 22
mixed session runtime, decoded stream runtime, raw tunnel/BodyRef runtime, and test migration in one slice; the user
approved splitting it on 2026-07-07. This task owns the first slice only.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (official worker rebased onto the SDK)
- `docs/planning/12-nats-protocol.md` (registration, heartbeat, exact-session assignment subject boundary)
- `docs/planning/16-egress-execution.md` (official outbound execution stays in the worker)
- `docs/planning/32-open-decisions.md` (superseded `P2 Provider Adapter Baseline` acceptance tests)

## Prerequisites

- Task 12 completed (`sdk/egress` foundation and `Executor` interface exist).

## Out of Scope

- Do not move decoded request stream framing, cancellation, or response credit; task 26 owns that.
- Do not move raw tunnel or BodyRef handling; task 27 owns that.
- Do not finish migrating all runtime tests or add final conformance/live verification; tasks 28 and 24 own that.
- Do not rename `DESTINATION_RESOLUTION_PROVIDER_ADAPTER`; task 23 owns that.
- Do not add the standalone custom Egress example; task 13 owns that.
- Do not add new execution behavior; the official outbound HTTP engine remains in `internal/egress`.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

## Expected Files

- Modify: `sdk/egress/` — add the live session runtime for registration, heartbeat, retry, session loss, ready/drain,
  and a temporary assignment-loop factory seam.
- Modify: `internal/egress` — remove or delegate session runtime code that now lives in `sdk/egress`; keep the
  official outbound executor and temporary assignment-loop implementation.
- Modify: `cmd/egress/main.go` — construct and run the `sdk/egress` session runtime with the internal executor and
  temporary assignment-loop factory.
- Test: SDK registration/retry/session-loss tests; `cmd/egress` wiring test proving the binary path invokes
  `sdk/egress.Run`.

## Steps

- [x] Read all required planning docs.
- [x] Move registration, heartbeat, bounded registration retry, session loss re-registration, ready/draining state, and
      graceful session stop from `internal/egress` to `sdk/egress` without changing NATS wire behavior.
- [x] Make `internal/egress.Executor` implement `sdk/egress.Executor`.
- [x] Rewire `cmd/egress` so the built `cmd/egress` binary constructs and runs the `sdk/egress` session runtime.
- [x] Keep exact-session assignment handling delegated through an explicit temporary factory seam owned by task 26.
- [x] Move/adapt existing registration, retry, and session-loss tests so SDK session behavior is covered under
      `sdk/egress`.
- [x] Add a `cmd/egress` wiring test proving the binary path uses the SDK runtime.
- [x] Verify `sdk/egress` imports no `internal/*` packages.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `cmd/egress/main.go` constructs the worker session through `sdk/egress.Run`, not `internal/egress.Run`.
- `internal/egress` no longer owns registration, heartbeat loop, bounded registration retry, or session-loss
  re-registration except as compatibility wrappers if required by existing tests.
- `sdk/egress` runtime tests cover registration retry with fresh nonces, retry cancellation, heartbeat NACK
  re-registration, ready state, and final draining heartbeat.
- Exact-session assignment subscription and request stream protocol remain owned by follow-on tasks 26-28, named in
  this task's handoff rather than left as an unowned deferral.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- Existing official executor tests still pass, proving outbound execution behavior stayed in `internal/egress` and did
  not change.

## Handoff Notes

- Record exactly what moved to `sdk/egress` and what stayed in `internal/egress`.
- Record the `cmd/egress` wiring evidence.
- State that tasks 26-28 still own decoded stream runtime, raw tunnel/BodyRef runtime, and full runtime test migration,
  and that task 24 still owns independent SDK conformance plus live compose verification.

## Stop Conditions

- Stop if the rebase cannot preserve wire behavior without protocol changes.
- Stop if `sdk/egress` would need to import `internal/*`.
- Stop if a deferral would have no owning task file.

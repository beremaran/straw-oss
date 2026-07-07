# 24 - Egress SDK Conformance and Live Verification

Status: not started

## Objective

Prove the rebased Egress SDK works without implementer assumptions: add an SDK-only stub-executor conformance test and
drive one real request through the compose stack against the `cmd/egress` binary that now runs on `sdk/egress`.

## Context (gap being closed)

Tasks 12 and 22 create and wire the SDK, but the P2 decision requires acceptance tests for "SDK-built worker protocol
conformance" and "official worker on the SDK passing the existing E2E flow" (`docs/planning/32-open-decisions.md`).
The original task 12 included both implementation and live verification, which made it too large to verify honestly.
This task owns the independent conformance/live proof after the rebase lands.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (SDK/custom Egress boundary)
- `docs/planning/12-nats-protocol.md` (registration, assignment, stream, error protocol)
- `docs/planning/30-testing-matrix.md` (Egress SDK test rows before feature ships)
- `docs/planning/32-open-decisions.md` (P2 Provider Adapter Baseline acceptance tests)

## Prerequisites

- Task 12 completed (`sdk/egress` foundation exists).
- Task 22 completed (`cmd/egress` session runtime is rebased onto `sdk/egress`).
- Task 26 completed (decoded stream runtime is rebased onto `sdk/egress`).
- Task 27 completed (raw tunnel runtime is rebased onto `sdk/egress`).
- Task 31 completed (BodyRef request-body runtime is rebased onto `sdk/egress`).
- Task 28 completed (runtime tests and command wiring proof are migrated).
- Task 23 completed (public docs/protobuf no longer use Provider Adapter terminology for executor-delegated mode).

## Out of Scope

- Do not add the example custom implementation; task 13 owns that after this proof passes.
- Do not change the SDK API except for defects that block the conformance test; if an API change is needed, keep it
  minimal and record it.
- Do not add new execution behavior to the official worker.

## Expected Files

- Test: `sdk/egress` conformance test using a stub executor and no `internal/*` imports.
- Test/Docs: live compose verification notes or script updates only if existing compose instructions need a narrow
  correction to drive the request.
- Modify: task/handoff notes that currently say SDK/live verification is pending, if any.

## Steps

- [ ] Read all required planning docs.
- [ ] Add an SDK-only stub-executor conformance test that registers, heartbeats, receives an assignment, streams a
      response, and maps an executor error without importing `internal/*`.
- [ ] Verify `sdk/egress` and the conformance test import no `internal/*` packages.
- [ ] Bring up `deploy/docker`, rebuild `egress`, and drive a real request through Control to the rebased worker.
- [ ] Record live request evidence, including the Control request result and any relevant compose service commands.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./sdk/...`
- live compose request through Control and `cmd/egress`
- `make check`

## Acceptance Criteria

- The SDK-only conformance test proves registration, heartbeat, assignment receipt, response streaming, and executor
  error mapping with no `internal/*` imports.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches, including conformance tests.
- A live request through the compose stack succeeds against the rebased `cmd/egress` binary.
- Task 13 can proceed using only `sdk/egress`; any blocker discovered here is either fixed in this task or assigned to
  a new owning task before handoff.

## Handoff Notes

- Include the conformance verdict table: registration, heartbeat, assignment, response stream, executor error.
- Include live compose commands and result.
- State whether task 13 can proceed purely against `sdk/egress`.

## Stop Conditions

- Stop if live verification fails for a reason outside the SDK/rebase work.
- Stop if the conformance test cannot be written without `internal/*` imports.
- Stop if a deferral would have no owning task file.

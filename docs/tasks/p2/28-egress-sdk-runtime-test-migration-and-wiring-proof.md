# 28 - Egress SDK Runtime Test Migration and Wiring Proof

Status: done

## Objective

Finish the official-worker rebase proof by moving or deleting stale `internal/egress` protocol-runtime tests, leaving
`internal/egress` focused on outbound execution, and adding a command-level wiring proof that `cmd/egress` constructs
the SDK runtime with the official executor.

## Context (gap being closed)

Tasks 22, 26, 27, and 31 move runtime behavior in slices. The original task 22 also required moving/adapting runtime tests
and proving `cmd/egress` wiring, but doing that before the runtime move is complete leaves duplicate tests and
ambiguous ownership. Current tests such as `internal/egress/loop_test.go`, `internal/egress/runtime_test.go`, and
`internal/egress/assignment_test.go` cover protocol machinery that belongs under `sdk/egress` once tasks 22, 26, 27,
and 31 are complete.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (official worker rebased onto SDK)
- `docs/planning/12-nats-protocol.md` (registration, assignment, stream, error protocol)
- `docs/planning/30-testing-matrix.md` (Egress SDK test rows before feature ships)

## Prerequisites

- Task 22 completed (session runtime moved).
- Task 26 completed (decoded stream runtime moved).
- Task 27 completed (raw tunnel runtime moved).
- Task 31 completed (BodyRef request-body runtime moved).

## Out of Scope

- Do not add live compose verification; task 24 owns it.
- Do not rename Provider Adapter terminology; task 23 owns it.
- Do not add a custom Egress example; task 13 owns it after task 24.

## Expected Files

- Modify: `sdk/egress/*_test.go` — complete migrated runtime coverage.
- Modify: `internal/egress/*_test.go` — remove or narrow tests that now belong to SDK runtime.
- Modify/Test: `cmd/egress/main.go` and `cmd/egress/main_test.go` only if a narrower injectable seam is needed for
  wiring proof.

## Steps

- [x] Read all required planning docs.
- [x] Move or delete stale internal protocol-runtime tests after confirming equivalent SDK tests exist.
- [x] Keep internal executor tests for outbound HTTP execution behavior.
- [x] Add/finish a `cmd/egress` wiring test proving the binary path constructs `sdk/egress.Run` with the official
      executor.
- [x] Verify `sdk/egress` imports no `internal/*` packages.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- Protocol-runtime coverage lives under `sdk/egress`; `internal/egress` tests cover official outbound execution and
  official-only adapters.
- `cmd/egress` has a test proving the built binary path invokes the SDK runtime rather than `internal/egress.Run`.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- Task 24 can proceed to independent SDK conformance and live compose verification.

## Handoff Notes

- Record the test files moved, narrowed, or deleted.
- State that task 24 owns live compose verification and SDK-only conformance.

## Stop Conditions

- Stop if equivalent SDK coverage cannot be established before deleting internal tests.
- Stop if a deferral would have no owning task file.

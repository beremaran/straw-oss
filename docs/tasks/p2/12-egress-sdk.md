# 12 - Egress SDK Protocol Foundation

Status: done

## Objective

Create the public `sdk/egress` package foundation for custom Egress implementations: public protocol types,
registration/heartbeat helpers, assignment admission, stream subject/envelope helpers, and the pluggable `Executor`
interface derived from the official worker's existing execution seam. This task deliberately stops before rebasing the
official worker so the public API boundary can be verified first.

## Context (gap being closed)

The 2026-07-07 decision `P2 Provider Adapter Baseline` (superseded entry in `docs/planning/32-open-decisions.md`)
dropped the Provider Adapter concept: provider integrations become custom Egress implementations built on an Egress
SDK. Current code has no public Egress SDK: `sdk/` contains only the client SDK, while worker protocol machinery is
inside unimportable `internal/egress` and `internal/natsx`. Evidence: `internal/egress/registration.go` defines
`Identity`, `Capabilities`, `BuildRegisterRequest`, `Register`, and `Heartbeat`; `internal/egress/assignment.go`
defines `Capacity` and `EvaluateAssignment`; `internal/egress/loop.go` defines the live `Worker`; `internal/natsx`
owns subject and envelope helpers. The official executor already exposes the intended seam:
`Execute(ctx, start, body, attempt, send)` in `internal/egress/executor.go`; the full extraction was too large for one
safe task, so follow-on tasks 22-24 own the rebase, enum rename, and conformance/live verification.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations)
- `docs/planning/12-nats-protocol.md` (subjects, envelope, registration, heartbeat, assignment, stream rules)
- `docs/planning/32-open-decisions.md` (superseded `P2 Provider Adapter Baseline` entry)

## Prerequisites

- None within P2 (the P0 worker loop, registration, and executor seam this task exposes already exist).

## Out of Scope

- Do not rebase `cmd/egress` or `internal/egress` onto `sdk/egress`; task 22 owns that.
- Do not rename `DESTINATION_RESOLUTION_PROVIDER_ADAPTER`; task 23 owns the protobuf/doc cleanup.
- Do not add the SDK conformance/live compose verification; task 24 owns that after the rebase.
- Do not add the example custom implementation; task 13 owns that after task 24.
- No marketplace discovery, provider billing, provider account provisioning, or new execution behavior.

## Expected Files

- Add: `sdk/egress/` — public worker SDK foundation: identity/capabilities, registration request construction,
  heartbeat construction, assignment admission, subject/envelope helpers needed by SDK consumers, and `Executor`.
- Modify: `internal/egress` only as needed to reuse public SDK types without changing official worker behavior.
- Test: SDK foundation unit tests for registration signing, inbox/subject validation, heartbeat construction,
  assignment admission, and stream envelope round trips.

## Steps

- [x] Read all required planning docs.
- [x] Define `sdk/egress.Executor` from the existing `Execute(ctx, start, body, attempt, send)` seam.
- [x] Move or wrap the public-safe identity, capability, registration-request, heartbeat, capacity, and assignment
      admission helpers into `sdk/egress`.
- [x] Expose SDK-safe subject and envelope helpers required for custom workers without importing `internal/*`
      packages.
- [x] Keep the official worker behavior unchanged; any `internal/egress` edits must be mechanical reuse of the new
      public helpers.
- [x] Add focused SDK foundation tests.
- [x] Verify `sdk/egress` imports no `internal/*` packages.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress`
- `make check`

## Acceptance Criteria

- `sdk/egress` exists and exposes `Executor`, identity/capability types, registration/heartbeat helpers, assignment
  admission, and subject/envelope helpers sufficient for a custom worker runtime foundation.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- SDK tests prove registration signing, heartbeat construction, assignment admission, and subject/envelope round trips.
- Existing `internal/egress` tests still pass, proving no official worker behavior changed in this foundation slice.
- Tasks 22, 23, and 24 own the remaining Egress SDK extraction work explicitly.

## Handoff Notes

- Record the public `Executor` interface shape and the SDK helpers added.
- Record any `internal/egress` behavior intentionally left untouched for task 22.
- State that task 13 remains blocked until task 24 is complete.

## Stop Conditions

- Stop if a public `sdk/egress` foundation cannot be built without importing `internal/*`.
- Stop if preserving current worker behavior requires protocol changes.
- Stop if a deferral would have no owning task file.

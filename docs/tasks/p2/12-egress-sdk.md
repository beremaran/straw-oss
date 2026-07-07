# 12 - Egress SDK

Status: not started

## Objective

Extract the Egress worker's protocol machinery — NATS registration, heartbeat, assignment handling, stream framing,
and error protocol — from `internal/egress` into a public `sdk/egress` package with a pluggable `Executor` interface,
and rebase the official worker (`cmd/egress`) onto it as the SDK's reference implementation. When done, a third party
can build a custom Egress node by importing `sdk/egress` and supplying only an `Executor`.

## Context (gap being closed)

The 2026-07-07 decision `P2 Provider Adapter Baseline` (superseded entry in `docs/planning/32-open-decisions.md`)
dropped the Provider Adapter concept: provider integrations become custom Egress implementations built on an Egress
SDK. That SDK does not exist. Current code: `sdk/` is the client SDK only; all worker protocol machinery lives in
`internal/egress` (unimportable outside the module); the loop takes the concrete executor, not an interface
(`NewWorker(conn, id, executor *Executor, ...)` at `internal/egress/loop.go:59`); the execution seam already exists as
one method, `Execute(ctx, start, body, attempt, send)` at `internal/egress/executor.go:387`; the binary wires it via
`egress.Run(ctx, natsConn, id, caps, executor, heartbeatInterval, ready)` at `cmd/egress/main.go:264`. This task also
owns the `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` naming cleanup flagged in `docs/planning/27-security-controls.md`
(Executor-delegated resolution section).

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (Egress Worker; Egress SDK and Custom Egress Implementations)
- `docs/planning/12-nats-protocol.md` (envelope, registration, heartbeat, stream, error protocol the SDK must carry)
- `docs/planning/13-protobuf-contract.md` (compatibility rules; reserved names/numbers; Buf breaking checks)
- `docs/planning/27-security-controls.md` (Executor-delegated resolution; destination policy modes)
- `docs/planning/32-open-decisions.md` (superseded `P2 Provider Adapter Baseline` entry)

## Prerequisites

- None within P2 (the P0 worker loop, registration, and executor this task extracts are complete).

## Out of Scope

- No marketplace discovery, provider billing, or provider account provisioning.
- No new execution behavior: the official worker's outbound execution engine stays in `internal/egress` and must
  behave identically after the rebase.
- No example custom implementation (task 13 owns that).
- No protocol changes beyond the enum-value rename; wire numbers and message shapes are untouched.

## Expected Files

- Add: `sdk/egress/` — public worker runtime (registration, heartbeat, assignment, stream framing, error protocol)
  and the `Executor` interface derived from the `Execute(ctx, start, body, attempt, send)` seam.
- Modify: `internal/egress` — keeps the official outbound execution engine (`Executor`), now implementing the SDK
  interface; protocol machinery moves out.
- Modify: `cmd/egress/main.go` — constructs the `sdk/egress` worker with the `internal/egress` executor.
- Modify: `api/proto/straw/v1/straw.proto` (+ regenerate) and `api/proto/straw/v1/validate.go:270` — rename
  `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` to executor-delegated naming, wire number unchanged, old name reserved
  per `13-protobuf-contract.md`.
- Modify: `docs/planning/27-security-controls.md` — drop the "rename owned by the P2 Egress SDK task" parenthetical.
- Test: moved/adapted loop, registration, and assignment tests under `sdk/egress`; a new conformance test driving the
  SDK worker with a stub executor; `cmd/egress` wiring test.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the `Executor` interface in `sdk/egress` from the existing `Execute(ctx, start, body, attempt, send)`
      seam.
- [ ] Move registration, heartbeat, assignment, stream-framing, and error-protocol code from `internal/egress` to
      `sdk/egress`, leaving the outbound execution engine in `internal/egress` as an implementation of the interface.
- [ ] Rewire `cmd/egress` so the binary constructs the `sdk/egress` worker with the internal executor.
- [ ] Rename the `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` enum value (wire number unchanged, old name reserved),
      regenerate, and update `validate.go` and the `27-security-controls.md` note.
- [ ] Move/adapt the existing protocol tests and add a stub-executor conformance test proving registration,
      assignment, stream frames, and error mapping work with no `internal/egress` code involved.
- [ ] Bring up `deploy/docker`, rebuild `egress`, and drive a real request through Control to the rebased worker.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `sdk/egress` exposes the worker runtime and `Executor` interface with no imports of `internal/*` packages (proven
  by `go list -deps` or grep over `sdk/egress` imports).
- `cmd/egress` constructs the `sdk/egress` worker (visible in `cmd/egress/main.go`), and a live request through the
  compose stack succeeds against the rebased binary.
- A stub-executor conformance test registers, heartbeats, receives an assignment, streams a response, and maps an
  executor error — exercising only `sdk/egress`.
- `grep -rn PROVIDER_ADAPTER` over non-generated code returns only the proto `reserved` entry for the old name, and
  Buf breaking checks pass.
- The `27-security-controls.md` "rename owned by" parenthetical is gone.

## Handoff Notes

- Record what stayed in `internal/egress` vs moved to `sdk/egress`, and the exact `Executor` interface shape.
- Record the new enum value name and confirmation that Buf breaking checks passed.
- State whether task 13 can proceed purely against `sdk/egress`.

## Stop Conditions

- Stop if Buf breaking checks reject the enum rename even with reserved old name; ask instead of forcing it.
- Stop if the extraction cannot preserve wire behavior without protocol changes.
- Stop if a deferral would have no owning task file.

# 31 - Egress SDK BodyRef Request-Body Runtime Rebase

Status: done

## Objective

Move the request-body `BodyRefFrame` runtime from `internal/egress` into `sdk/egress`: BodyRef scope validation and the
download/verification hooks (checksum, expiry, unavailable-object mapping) must run through the public SDK. The official
worker's BodyRef HTTP client stays in `internal/egress` behind a minimal SDK body-ref interface.

## Context (gap being closed)

The original task 22 was split on 2026-07-07. Its raw-tunnel/BodyRef follow-on was itself too large, so on 2026-07-07
it was split again: task 27 owns the raw CONNECT tunnel move, and this task owns the BodyRef request-body move. The two
are independent surfaces with independent tests.

Current code splits BodyRef handling between `internal/egress/loop.go` — `acceptBodyRef` (`loop.go:674`) and its
object-key scope check invoking `downloadBodyRef` (`loop.go:685`) — and `internal/egress/executor.go` —
`downloadBodyRef` (`executor.go:455`) with checksum/expiry verification. Custom SDK workers cannot implement the
request-body protocol until these hooks are public and internal-free.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations)
- `docs/planning/12-nats-protocol.md` (BodyRef frame, stream sequencing)
- `docs/planning/16-egress-execution.md` (official outbound execution stays in the worker)
- `docs/planning/18-large-body-transport-p2.md` (BodyRef request-body semantics)

## Prerequisites

- Task 27 completed (SDK owns raw tunnel runtime; both tasks edit `loop.go`, so serialize to avoid conflicts).

## Out of Scope

- Do not move raw CONNECT tunnel runtime; task 27 owns it.
- Do not move decoded stream runtime; task 26 owns it.
- Do not change object-storage server behavior or lifecycle rules; tasks 06-08 and 21 own those.
- Do not add final SDK conformance/live verification; task 24 owns that after tasks 22, 23, 26, 27, 28, and 31.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

## Expected Files

- Modify: `sdk/egress/` — add BodyRef scope validation and download/verification hooks behind a minimal body-ref
  interface.
- Modify: `internal/egress` — keep the official BodyRef HTTP client behind the SDK interface; remove moved BodyRef
  protocol machinery from `loop.go`.
- Test: SDK BodyRef runtime tests covering scope validation, checksum mismatch, expiry, and unavailable-object error
  mapping.

## Steps

- [x] Read all required planning docs.
- [x] Move BodyRef scope validation and download/verification hooks into `sdk/egress` behind a minimal body-ref
      interface.
- [x] Keep the official BodyRef HTTP client behavior in `internal/egress`.
- [x] Add/move the BodyRef tests listed in Expected Files.
- [x] Verify `sdk/egress` imports no `internal/*` packages.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `internal/egress` no longer owns request-body BodyRef stream protocol except through the SDK-facing official body-ref
  hook.
- SDK tests prove BodyRef scope validation, checksum mismatch, expiry, and unavailable-object error mapping.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- Existing official executor tests still pass.

## Handoff Notes

- Record the SDK body-ref interface and what official-worker BodyRef behavior remains internal.
- State whether task 28 can remove any temporary compatibility wrappers.

## Stop Conditions

- Stop if `sdk/egress` would need to import `internal/*`.
- Stop if preserving wire behavior requires protocol changes.
- Stop if a deferral would have no owning task file.

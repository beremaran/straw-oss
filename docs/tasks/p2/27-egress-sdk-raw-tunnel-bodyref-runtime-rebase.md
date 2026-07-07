# 27 - Egress SDK Raw Tunnel and BodyRef Runtime Rebase

Status: not started

## Objective

Move the remaining request-stream runtime pieces from `internal/egress` into `sdk/egress`: raw CONNECT tunnel upload
and download streaming, upload/download credit for tunnel frames, tunnel cancellation, request-body `BodyRefFrame`
scope validation, and BodyRef download/verification hooks. The official worker's actual dialer and BodyRef HTTP client
remain in `internal/egress` behind SDK interfaces.

## Context (gap being closed)

The original task 22 split left raw tunnel and BodyRef-specific runtime out of the decoded stream slice. Current code
keeps raw tunnel behavior in `internal/egress/loop.go` (`runRawTunnel`, `rawTunnelStream`, upload credit, cancellation)
and request-body BodyRef handling split between `internal/egress/loop.go` (`acceptBodyRef`, object-key scope check) and
`internal/egress/executor.go` (`downloadBodyRef`, checksum/expiry verification). Custom SDK workers cannot implement
the complete protocol until these hooks are public and internal-free.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations)
- `docs/planning/12-nats-protocol.md` (raw stream directions, credit, cancellation)
- `docs/planning/16-egress-execution.md` (official outbound execution stays in the worker)
- `docs/planning/18-large-body-transport-p2.md` (BodyRef request-body semantics)

## Prerequisites

- Task 26 completed (SDK owns decoded assignment and stream runtime).

## Out of Scope

- Do not change object-storage server behavior or lifecycle rules; tasks 06-08 and 21 own those.
- Do not add final SDK conformance/live verification; task 24 owns that after tasks 22, 23, 26, 27, and 28.
- Do not add a custom Egress example; task 13 owns that after task 24.

## Expected Files

- Modify: `sdk/egress/` — add raw tunnel runtime and public BodyRef hook interfaces.
- Modify: `internal/egress` — keep official dial/BodyRef implementations behind SDK interfaces; remove moved protocol
  machinery from `loop.go`.
- Test: SDK raw tunnel and BodyRef runtime tests covering tunnel data, upload credit, cancellation, BodyRef scope,
  checksum mismatch, expiry, and unavailable object mapping.

## Steps

- [ ] Read all required planning docs.
- [ ] Move raw tunnel stream runtime into `sdk/egress` behind a minimal dial/open interface.
- [ ] Move BodyRef scope validation and download/verification hooks into `sdk/egress` behind a minimal body-ref
      interface.
- [ ] Keep official outbound HTTP, dial target validation, and BodyRef HTTP client behavior in `internal/egress`.
- [ ] Add/move the raw tunnel and BodyRef tests listed in Expected Files.
- [ ] Verify `sdk/egress` imports no `internal/*` packages.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `internal/egress` no longer owns raw tunnel stream protocol or request-body BodyRef stream protocol except through
  SDK-facing official executor hooks.
- SDK tests prove raw tunnel data flow, upload credit, cancellation, BodyRef scope validation, checksum mismatch,
  expiry, and unavailable object error mapping.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- Existing official executor tests still pass.

## Handoff Notes

- Record the SDK hook interfaces and what official-worker behavior remains internal.
- State whether task 28 can remove any temporary compatibility wrappers.

## Stop Conditions

- Stop if `sdk/egress` would need to import `internal/*`.
- Stop if preserving wire behavior requires protocol changes.
- Stop if a deferral would have no owning task file.

# 27 - Egress SDK Raw Tunnel Runtime Rebase

Status: done

## Objective

Move the raw CONNECT tunnel request-stream runtime from `internal/egress` into `sdk/egress`: raw tunnel upload and
download streaming, upload/download credit for tunnel frames, and tunnel cancellation must run through the public SDK.
The official worker's actual dialer stays in `internal/egress` behind a minimal SDK dial/open interface.

## Context (gap being closed)

The original task 22 was split on 2026-07-07 because it mixed the whole worker runtime in one oversized slice. That
split originally left raw tunnel and BodyRef runtime together in one follow-on task, which was itself too large: raw
CONNECT tunnel streaming and request-body BodyRef download/verification are independent surfaces with independent tests.
This task owns the raw tunnel move only; task 31 owns the BodyRef request-body runtime move.

Current code keeps raw tunnel behavior in `internal/egress/loop.go`: `runRawTunnel` (`loop.go:378`), the
`rawTunnelStream` type and its `run`/`handleFrame`/`handleData`/`publish` methods (`loop.go:405-496`), plus upload
credit and cancellation. Custom SDK workers cannot implement the raw tunnel protocol until these hooks are public and
internal-free.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md` (Egress SDK and Custom Egress Implementations)
- `docs/planning/12-nats-protocol.md` (raw stream directions, credit, cancellation)
- `docs/planning/16-egress-execution.md` (official outbound execution stays in the worker)

## Prerequisites

- Task 26 completed (SDK owns decoded assignment and stream runtime).

## Out of Scope

- Do not move BodyRef request-body download/verification hooks; task 31 owns those.
- Do not move decoded stream runtime; task 26 owns it.
- Do not add final SDK conformance/live verification; task 24 owns that after tasks 22, 23, 26, 27, 28, and 31.
- Do not change NATS wire messages, subject shapes, stream sequencing, or error semantics.

## Expected Files

- Modify: `sdk/egress/` — add raw tunnel runtime behind a minimal dial/open interface.
- Modify: `internal/egress/loop.go` — keep the official dialer behind the SDK interface; remove moved raw tunnel
  protocol machinery.
- Test: SDK raw tunnel runtime tests covering tunnel data flow, upload credit, and cancellation.

## Steps

- [x] Read all required planning docs.
- [x] Move raw tunnel stream runtime into `sdk/egress` behind a minimal dial/open interface.
- [x] Keep official outbound dial and dial-target validation in `internal/egress`.
- [x] Add/move the raw tunnel tests listed in Expected Files.
- [x] Verify `sdk/egress` imports no `internal/*` packages.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./sdk/... ./internal/egress ./cmd/egress`
- `make check`

## Acceptance Criteria

- `internal/egress` no longer owns raw tunnel stream protocol except through the SDK-facing official dial interface.
- SDK tests prove raw tunnel data flow, upload credit, and cancellation.
- `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches.
- Existing official executor tests still pass.

## Handoff Notes

- Record the SDK dial/open interface and what official-worker dial behavior remains internal.
- State that task 31 still owns BodyRef request-body runtime movement.

## Stop Conditions

- Stop if `sdk/egress` would need to import `internal/*`.
- Stop if preserving wire behavior requires protocol changes.
- Stop if a deferral would have no owning task file.

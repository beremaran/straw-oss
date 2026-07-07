# 30 - Ingress HTTP/2 Upload Flow Control and Live Proof

Status: done

## Objective

Complete the final slice of the task 14 ingress HTTP/2 semantics: ingress upload flow control so exhausted NATS upload
credit applies HTTP/2 backpressure without unbounded buffering, and one live HTTP/2 MITM request driven through the
compose stack via the normal Control -> NATS -> Egress stream protocol, proving end-to-end protocol translation.

## Context (gap being closed)

The original combined task 25 bundled the full ingress HTTP/2 semantics into one slice that two verifiers rejected as
too large. It was split on 2026-07-07 into three: task 25 (stream identity + cancellation), task 29 (headers +
trailers), and this task (upload flow control + live proof). This is the last slice; it also carries the live
protocol-translation proof for all three because that proof exercises identity, cancellation, and headers together.

Current code terminates ingress h2 via Go's `net/http` h2 server in `internal/control/mitm_connect_handler.go` and
dispatches through `internal/control/mitm_handler.go` and `internal/control/dispatcher.go`, without ingress-h2 upload
credit backpressure tests or a recorded live HTTP/2 MITM request through the normal stream path.

## Required Planning Docs

- `docs/planning/12-nats-protocol.md` (credit semantics, stream ordering)
- `docs/planning/15-http-semantics.md` (HTTP/2 flow-control prerequisite list)
- `docs/planning/17-mitm-design-p2.md` (MITM TLS termination boundary)
- `docs/planning/c-http2-semantics.md` (task 14 ingress flow-control row and normal stream translation)
- `docs/planning/30-testing-matrix.md` (HTTP/2 implementation test rows)

## Prerequisites

- Task 25 completed (per-stream `request_id`, cancellation, connection fanout).
- Task 29 completed (pseudo-header normalization and trailer handling), so the live request exercises the full ingress
  contract.

## Out of Scope

- Do not re-implement stream identity, cancellation, or header/trailer semantics; tasks 25 and 29 own those.
- Do not change outbound HTTP/2 or egress downgrade behavior; task 15 owns that.
- Do not change MITM CA, leaf generation, or leaf cache behavior.
- Do not change the NATS protocol contract to make flow control work; that is a stop condition.

## Expected Files

- Modify/Test: `internal/control/mitm_handler.go`, `internal/control/mitm_connect_handler.go`,
  `internal/control/dispatcher.go`, and focused tests for ingress upload flow control.
- Test/Docs: live compose verification notes or scripts only if existing compose instructions need a narrow correction
  to drive the HTTP/2 MITM request.

## Steps

- [x] Read all required planning docs.
- [x] Build a coverage table for the flow-control and normal-stream-translation rows in
      `docs/planning/c-http2-semantics.md`.
- [x] Implement and test ingress upload flow control so exhausted NATS upload credit stops or slows client h2
      reads/window updates without unbounded buffering.
- [x] Drive at least one HTTP/2 MITM request through the compose stack across Control -> NATS -> Egress, proving
      protocol translation through the normal stream protocol.
- [x] Record live compose commands and the request result.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- Focused Control/MITM HTTP/2 flow-control tests (`go test ./internal/control`).
- Live compose HTTP/2 MITM request through Control -> NATS -> Egress.
- `make check`

## Acceptance Criteria

- NATS upload-credit exhaustion applies ingress HTTP/2 backpressure without unbounded buffering, proven by a focused
  test.
- A live HTTP/2 MITM request succeeds through the compose stack via the normal Control -> NATS -> Egress stream
  protocol, with commands and result recorded in the handoff.

## Handoff Notes

- Include the flow-control and normal-stream-translation coverage rows for `docs/planning/c-http2-semantics.md`.
- Record live compose commands and the request result.
- State whether any remaining HTTP/2 behavior is out of phase, and name the owning task if so.

## Stop Conditions

- Stop if implementing the flow-control behavior requires changing the NATS protocol contract from
  `docs/planning/12-nats-protocol.md`.
- Stop if live verification fails for a reason outside ingress HTTP/2.
- Stop if a deferral would have no owning task file.

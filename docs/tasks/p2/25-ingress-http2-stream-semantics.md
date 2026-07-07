# 25 - Ingress HTTP/2 Stream Semantics

Status: not started

## Objective

Complete the ingress HTTP/2 stream semantics from task 14 that were deliberately split out of task 16: one Straw
`request_id` per HTTP/2 stream, client-stream cancellation mapping, NATS-credit-aware upload flow control,
pseudo-header and trailer behavior, connection-level error fanout, and live protocol-translation proof through the
normal Control -> NATS -> Egress path.

## Context (gap being closed)

Task 16 originally mixed MITM ALPN enablement with the full HTTP/2 ingress semantics from
`docs/planning/c-http2-semantics.md`. Two independent verifiers rejected marking task 16 done because the implemented
diff only proves policy-gated MITM ALPN plus basic h2 MITM requests; it does not prove cancellation, NATS-credit
flow-control, trailers, connection-level fanout, or live NATS/egress protocol translation. Current code relies on
Go's `net/http` h2 server setup in `internal/control/mitm_connect_handler.go`, while request dispatch still flows
through the existing decoded MITM handler and dispatcher without ingress-h2-specific cancellation/credit/trailer/fanout
tests.

## Required Planning Docs

- `docs/planning/15-http-semantics.md` (HTTP/2 prerequisite list)
- `docs/planning/12-nats-protocol.md` (stream ordering, cancellation frames, credit semantics, trailer frames)
- `docs/planning/17-mitm-design-p2.md` (MITM TLS termination boundary)
- `docs/planning/c-http2-semantics.md` (task 14 ingress and MITM HTTP/2 semantics)
- `docs/planning/30-testing-matrix.md` (HTTP/2 implementation test rows)

## Prerequisites

- Task 16 completed (policy-gated MITM ALPN and basic h2 MITM request path exist).
- Task 15 completed (outbound HTTP/2 and downgrade behavior are already owned outside this task).

## Out of Scope

- Do not change outbound HTTP/2 or egress downgrade behavior; task 15 owns that.
- Do not change MITM CA, leaf generation, or leaf cache behavior.
- Do not add a new tenant HTTP/2 policy surface unless the required planning docs are updated first; task 16 uses the
  existing MITM routing-policy gate.

## Expected Files

- Modify/Test: `internal/control/mitm_connect_handler.go`, `internal/control/mitm_handler.go`,
  `internal/control/dispatcher.go`, and focused tests as needed for ingress HTTP/2 stream semantics.
- Test: Control/MITM HTTP/2 tests proving cancellation, flow-control interaction, pseudo-header/trailer behavior,
  connection-level fanout, and normal stream protocol translation.
- Test/Docs: live compose verification notes or scripts only if existing instructions need a narrow correction.

## Steps

- [ ] Read all required planning docs.
- [ ] Build a coverage table for every ingress-owned row in `docs/planning/c-http2-semantics.md`.
- [ ] Prove each HTTP/2 stream maps to one unique Straw `request_id` through Control admission and dispatch.
- [ ] Implement and test client stream reset/disconnect mapping to NATS `CancelFrame` for the affected request only.
- [ ] Implement and test ingress upload flow-control behavior so exhausted NATS upload credit stops or slows client h2
      reads/window updates without unbounded buffering.
- [ ] Verify pseudo-header normalization and rejection of unsafe colon-prefixed headers at ingress.
- [ ] Verify HTTP/2 trailers are forwarded or recorded according to the ingress contract and NATS `TrailersFrame`
      ordering.
- [ ] Verify a client HTTP/2 connection-level failure cancels all active streams and publishes per-request cancels.
- [ ] Drive at least one HTTP/2 MITM request through the compose stack across Control -> NATS -> Egress, proving
      protocol translation through the normal stream protocol.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- Focused Control/MITM HTTP/2 tests.
- Live compose HTTP/2 MITM request through Control -> NATS -> Egress.
- `make check`

## Acceptance Criteria

- Concurrent HTTP/2 ingress streams each receive a unique `request_id`, and cancellation of one stream does not cancel
  sibling streams.
- Client HTTP/2 stream reset/disconnect publishes a NATS `CancelFrame` for the matching request.
- NATS upload-credit exhaustion applies ingress HTTP/2 backpressure without unbounded buffering, proven by a focused
  test.
- Pseudo-headers are normalized as specified by task 14, and unsafe custom colon-prefixed headers are rejected or
  stripped according to the spec.
- HTTP/2 trailers follow the task 14/NATS `TrailersFrame` ordering contract.
- HTTP/2 client connection-level failure fans out cancellation to all active in-flight streams.
- A live HTTP/2 MITM request succeeds through the compose stack via the normal Control -> NATS -> Egress stream
  protocol.

## Handoff Notes

- Include a per-row coverage table for `docs/planning/c-http2-semantics.md` ingress-owned semantics.
- Record live compose commands and the request result.
- State whether any remaining HTTP/2 behavior is out of phase, and name the owning task if so.

## Stop Conditions

- Stop if implementing the flow-control behavior requires changing the NATS protocol contract from
  `docs/planning/12-nats-protocol.md`.
- Stop if live verification fails for a reason outside ingress HTTP/2.
- Stop if a deferral would have no owning task file.

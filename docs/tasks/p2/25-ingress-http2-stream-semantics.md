# 25 - Ingress HTTP/2 Stream Identity and Cancellation

Status: done

## Objective

Complete the stream-identity and cancellation slice of the task 14 ingress HTTP/2 semantics that were split out of
task 16: each ingress HTTP/2 stream maps to exactly one Straw `request_id`, a client stream reset/disconnect maps to a
NATS `CancelFrame` for that request only, and a client HTTP/2 connection-level failure fans out cancellation to every
active in-flight stream on that connection.

## Context (gap being closed)

Task 16 originally mixed MITM ALPN enablement with the full HTTP/2 ingress semantics from
`docs/planning/c-http2-semantics.md`. Two independent verifiers rejected marking task 16 done because the implemented
diff only proves policy-gated MITM ALPN plus basic h2 MITM requests. The original combined task 25 then bundled
identity, cancellation, flow-control, pseudo-headers, trailers, connection fanout, and live proof into one slice — too
large for one honest run. That combined task was split on 2026-07-07 into three: this task (identity + cancellation),
task 29 (headers + trailers), and task 30 (upload flow control + live proof).

Current code relies on Go's `net/http` h2 server setup in `internal/control/mitm_connect_handler.go`, while request
dispatch flows through the decoded MITM handler (`internal/control/mitm_handler.go`) and the dispatcher
(`internal/control/dispatcher.go`) without ingress-h2-specific per-stream request-id, cancellation, or
connection-fanout tests.

## Required Planning Docs

- `docs/planning/12-nats-protocol.md` (stream ordering, cancellation frames)
- `docs/planning/17-mitm-design-p2.md` (MITM TLS termination boundary)
- `docs/planning/c-http2-semantics.md` (task 14 ingress HTTP/2 semantics — stream identity, cancellation, connection
  fanout rows)
- `docs/planning/30-testing-matrix.md` (HTTP/2 implementation test rows)

## Prerequisites

- Task 16 completed (policy-gated MITM ALPN and basic h2 MITM request path exist).

## Out of Scope

- Do not implement pseudo-header normalization, unsafe colon-header handling, or trailer forwarding; task 29 owns those.
- Do not implement NATS-credit upload flow control or the live compose HTTP/2 proof; task 30 owns those.
- Do not change outbound HTTP/2 or egress downgrade behavior; task 15 owns that.
- Do not change MITM CA, leaf generation, or leaf cache behavior.
- Do not add a new tenant HTTP/2 policy surface unless the required planning docs are updated first; task 16 uses the
  existing MITM routing-policy gate.

## Expected Files

- Modify/Test: `internal/control/mitm_connect_handler.go`, `internal/control/mitm_handler.go`,
  `internal/control/dispatcher.go`, and focused tests as needed for stream identity and cancellation.
- Test: Control/MITM HTTP/2 tests proving unique per-stream `request_id`, per-stream cancel isolation, and
  connection-level cancellation fanout.

## Steps

- [x] Read all required planning docs.
- [x] Build a coverage table for the stream-identity, cancellation, and connection-fanout rows in
      `docs/planning/c-http2-semantics.md` (leave header/trailer and flow-control rows to tasks 29 and 30).
- [x] Prove each HTTP/2 stream maps to one unique Straw `request_id` through Control admission and dispatch.
- [x] Implement and test client stream reset/disconnect mapping to NATS `CancelFrame` for the affected request only,
      leaving sibling streams uncancelled.
- [x] Implement and test that a client HTTP/2 connection-level failure cancels all active streams and publishes a
      per-request cancel for each.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- Focused Control/MITM HTTP/2 tests (`go test ./internal/control`).
- `make check`

## Acceptance Criteria

- Concurrent HTTP/2 ingress streams each receive a unique `request_id`, proven by a focused test.
- Cancellation of one HTTP/2 stream publishes a NATS `CancelFrame` for the matching request and does not cancel
  sibling streams, proven by a focused test.
- A client HTTP/2 connection-level failure fans out cancellation to all active in-flight streams on that connection,
  proven by a focused test.

## Handoff Notes

- Include a per-row coverage table for the identity/cancellation/fanout rows of `docs/planning/c-http2-semantics.md`.
- State that task 29 owns pseudo-headers/trailers and task 30 owns flow control plus the live compose proof.

## Stop Conditions

- Stop if implementing cancellation requires changing the NATS protocol contract from
  `docs/planning/12-nats-protocol.md`.
- Stop if a deferral would have no owning task file.

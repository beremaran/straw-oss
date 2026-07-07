# 29 - Ingress HTTP/2 Headers and Trailers

Status: not started

## Objective

Complete the header and trailer slice of the task 14 ingress HTTP/2 semantics: pseudo-headers are normalized as
specified by task 14, unsafe custom colon-prefixed headers are rejected or stripped, and HTTP/2 trailers are forwarded
or recorded following the task 14 / NATS `TrailersFrame` ordering contract.

## Context (gap being closed)

The original combined task 25 bundled the full ingress HTTP/2 semantics into one slice that two verifiers rejected as
too large. It was split on 2026-07-07 into three: task 25 (stream identity + cancellation), this task (headers +
trailers), and task 30 (upload flow control + live proof). This task owns only the header/trailer semantics.

Current code terminates ingress h2 via Go's `net/http` h2 server in `internal/control/mitm_connect_handler.go` and
dispatches through `internal/control/mitm_handler.go`, without ingress-h2-specific pseudo-header normalization,
colon-header rejection, or trailer-ordering tests.

## Required Planning Docs

- `docs/planning/15-http-semantics.md` (HTTP/2 header handling)
- `docs/planning/12-nats-protocol.md` (trailer frames, ordering)
- `docs/planning/c-http2-semantics.md` (task 14 ingress pseudo-header and trailer rows)
- `docs/planning/30-testing-matrix.md` (HTTP/2 implementation test rows)

## Prerequisites

- Task 25 completed (per-stream `request_id` and cancellation exist, so header/trailer work maps onto a known stream
  identity).

## Out of Scope

- Do not implement or change stream identity, cancellation, or connection fanout; task 25 owns those.
- Do not implement NATS-credit upload flow control or the live compose HTTP/2 proof; task 30 owns those.
- Do not change outbound HTTP/2 or egress downgrade behavior; task 15 owns that.
- Do not add a new tenant HTTP/2 policy surface unless the required planning docs are updated first.

## Expected Files

- Modify/Test: `internal/control/mitm_handler.go`, `internal/control/mitm_connect_handler.go`, and focused tests as
  needed for pseudo-header normalization, colon-header handling, and trailer forwarding.
- Test: Control/MITM HTTP/2 tests proving pseudo-header normalization, unsafe colon-header rejection/stripping, and
  trailer ordering.

## Steps

- [ ] Read all required planning docs.
- [ ] Build a coverage table for the pseudo-header and trailer rows in `docs/planning/c-http2-semantics.md`.
- [ ] Verify pseudo-header normalization at ingress matches the task 14 contract.
- [ ] Verify rejection or stripping of unsafe custom colon-prefixed headers at ingress.
- [ ] Verify HTTP/2 trailers are forwarded or recorded per the ingress contract and NATS `TrailersFrame` ordering.
- [ ] Run focused tests, then `make check`.
- [ ] Write a handoff note.

## Tests

- Focused Control/MITM HTTP/2 tests (`go test ./internal/control`).
- `make check`

## Acceptance Criteria

- Ingress HTTP/2 pseudo-headers are normalized as specified by task 14, proven by a focused test.
- Unsafe custom colon-prefixed headers are rejected or stripped according to the spec, proven by a focused test.
- HTTP/2 trailers follow the task 14 / NATS `TrailersFrame` ordering contract, proven by a focused test.

## Handoff Notes

- Include a per-row coverage table for the pseudo-header and trailer rows of `docs/planning/c-http2-semantics.md`.
- State that task 30 owns flow control plus the live compose proof.

## Stop Conditions

- Stop if forwarding trailers requires changing the NATS protocol contract from `docs/planning/12-nats-protocol.md`.
- Stop if a deferral would have no owning task file.

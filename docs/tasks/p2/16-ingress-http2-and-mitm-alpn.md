# 16 - Ingress HTTP/2 and MITM ALPN

Status: not started

## Objective

Implement ingress HTTP/2 stream mapping and MITM ALPN behavior if task 14 specifies them as supported.

## Required Planning Docs

- `docs/planning/15-http-semantics.md`
- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/12-nats-protocol.md`

## Prerequisites

- Task 14 completed.
- Task 02 completed if ALPN covers MITM.
- Task 04 completed if ALPN covers MITM.
- Task 20 completed if ALPN tests depend on cached MITM leaf selection.

## Out of Scope

- Do not implement outbound HTTP/2.
- Do not enable ALPN behavior not specified by task 14.
- Do not change HTTP/1.1 ingress behavior.

## Expected Files

- Create or modify: HTTP/2 ingress and MITM ALPN handling.
- Test: ingress HTTP/2 tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Confirm task 14 includes ingress and MITM ALPN support.
- [ ] Map each HTTP/2 stream to one `request_id`.
- [ ] Apply cancellation and flow-control semantics from task 14.
- [ ] Normalize pseudo-headers and trailers.
- [ ] Handle connection-level errors across active streams.
- [ ] Add tests for concurrent streams, cancellation, flow-control, pseudo-headers, trailers, connection errors, and MITM
      ALPN.
- [ ] Run focused ingress HTTP/2 tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused ingress HTTP/2 tests.
- `make check`

## Acceptance Criteria

- Ingress HTTP/2 follows task 14 semantics.
- MITM ALPN behavior is implemented only if specified.
- HTTP/1.1 ingress remains compatible.

## Handoff Notes

- Document supported ingress modes and ALPN behavior.

## Stop Conditions

- Stop if task 14 excludes or does not define this behavior.
- Stop if a deferral would have no owning task file.

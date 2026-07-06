# 05 - Raw CONNECT Tunnel

Status: done

## Objective

Implement P1 raw CONNECT tunnel ingress on port 8082 using existing NATS stream credit semantics and destination policy
validation.

## Required Planning Docs

- `docs/planning/15-http-semantics.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/27-security-controls.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/28-deployment.md`

## Prerequisites

- Task 02 completed.
- Task 04 completed.
- Task 24 completed (the "existing c2e/e2c credit protocol" this task relies on is only real once egress chunks
  by credit and Control replenishes).

## Out of Scope

- Do not allow CONNECT on the REST endpoint.
- Do not add SOCKS5, WebSockets, generic TCP, UDP, or QUIC.
- Do not implement MITM.

## Expected Files

- Create or modify: CONNECT ingress and tunnel streaming components.
- Modify: `cmd/control/main.go`
- Test: CONNECT tunnel tests.

## Steps

- [x] Read all required planning docs.
- [x] Specify and implement tunnel idle timeout and bandwidth accounting before accepting CONNECT traffic.
- [x] Add the port 8082 CONNECT listener behind static config.
- [x] Authenticate and authorize CONNECT requests.
- [x] Normalize and validate CONNECT target host/port with the Section 27 deny rules.
- [x] Carry tunnel bytes as request/response `DataFrame`s using the existing c2e/e2c credit protocol.
- [x] Reject CONNECT on REST and non-CONNECT methods on the raw CONNECT listener.
- [x] Add tests for denial normalization, credit/backpressure, idle timeout, cancellation, and REST rejection.
- [x] Run focused CONNECT tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused CONNECT tunnel tests.
- `make check`

## Acceptance Criteria

- CONNECT is accepted only on the P1 CONNECT ingress.
- Tunnel bytes obey existing NATS credit and cancellation rules.
- Future-work tunnel types are not generalized into this task.

## Handoff Notes

- Document tunnel timeout and accounting choices.

## Stop Conditions

- Stop before adding generic TCP/UDP/QUIC scope.
- Stop if tunnel framing would require a protobuf change not owned by this task.
- Stop if a deferral would have no owning task file.

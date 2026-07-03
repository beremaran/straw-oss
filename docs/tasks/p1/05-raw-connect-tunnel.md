# 05 - Raw CONNECT Tunnel

Status: not started

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

## Out of Scope

- Do not allow CONNECT on the REST endpoint.
- Do not add SOCKS5, WebSockets, generic TCP, UDP, or QUIC.
- Do not implement MITM.

## Expected Files

- Create or modify: CONNECT ingress and tunnel streaming components.
- Modify: `cmd/control/main.go`
- Test: CONNECT tunnel tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Specify and implement tunnel idle timeout and bandwidth accounting before accepting CONNECT traffic.
- [ ] Add the port 8082 CONNECT listener behind static config.
- [ ] Authenticate and authorize CONNECT requests.
- [ ] Normalize and validate CONNECT target host/port with the Section 27 deny rules.
- [ ] Carry tunnel bytes as request/response `DataFrame`s using the existing c2e/e2c credit protocol.
- [ ] Reject CONNECT on REST and non-CONNECT methods on the raw CONNECT listener.
- [ ] Add tests for denial normalization, credit/backpressure, idle timeout, cancellation, and REST rejection.
- [ ] Run focused CONNECT tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

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

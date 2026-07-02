# 10 - Assignment and Stream Lifecycle

Status: in progress

## Objective

Implement Control-to-Egress assignment and stream frame lifecycle with sequence, offset, credit, timeout, terminal, cancellation, and fallback rules.

## Required Planning Docs

- `docs/planning/08-request-id-and-trace-lifecycle.md`
- `docs/planning/09-canonical-request-lifecycle.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 03 completed.
- Task 06 completed.
- Task 09 completed.

## Out of Scope

- Do not implement Egress outbound HTTP internals beyond a fake executor needed for tests.
- Do not add silent request replay.
- Do not add durable queue behavior.

## Expected Files

- Create or modify: `internal/control`
- Create or modify: `internal/egress`
- Create or modify: `internal/natsx`
- Test: assignment and stream lifecycle tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement assignment accept and reject handling.
- [ ] Enforce ack timeout.
- [ ] Validate stream frame sequence, duplicates, out-of-order frames, offset mismatch, credits, and idle timeout.
- [ ] Validate `ErrorFrame` codes against the executor-emittable set (Section 13); out-of-set codes map to `executor_internal_error` and count toward worker cooldown.
- [ ] Implement terminal handling, including duplicate terminal ignored/counted and late frames ignored.
- [ ] Implement cancellation for client disconnect, deadline, and admin cancel; admin cancel with a tenant-scoped key requires the request to belong to the caller's tenant (`insufficient_permissions` otherwise, without confirming existence), while `system_admin` may cancel any request.
- [ ] Implement fallback only before `RequestStart`; do not silently replay after `RequestStart`.
- [ ] Add tests for every assignment, streaming, terminal, cancellation, fallback, NATS ordering, and timeout row in the testing matrix.
- [ ] Run focused lifecycle tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused lifecycle tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Assignment is exact-session and never duplicate retried.
- Stream validation rejects gaps, duplicates, out-of-order frames, offset mismatch, and credit exhaustion.
- Cancellation and terminal rules match the canonical lifecycle.
- Tests cover all lifecycle rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document lifecycle states and timeout constants.
- Note how fake NATS/executor behavior is tested.

## Stop Conditions

- Stop before adding durable queues or replay.
- Stop if lifecycle state conflicts with protobuf contract.

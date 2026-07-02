# 03 - NATS Connection and Subjects

Status: done

## Objective

Add the P0 NATS connection layer, startup max-payload validation, and exact-session subject helpers.

## Required Planning Docs

- `docs/planning/11-worker-discovery-and-health.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 02 completed.
- Generated protobuf types compile.

## Out of Scope

- Do not implement worker registration state machine.
- Do not dispatch real outbound requests.
- Do not use queue groups for executor dispatch.
- Do not introduce durable queues or replay behavior.

## Expected Files

- Create or modify: `internal/natsx`
- Modify: `cmd/control/main.go`
- Modify: `cmd/egress/main.go`
- Test: `internal/natsx/*_test.go`

## Steps

- [x] Read all required planning docs.
- [x] Define subject-building helpers for registration, heartbeat, assignment, stream frames, and terminal frames.
- [x] Add validation for safe subject tokens.
- [x] Add startup validation for NATS max payload against configured frame/body limits.
- [x] Add tests for exact assignment subject, no pool queue dispatch, unsafe token rejection, and max payload validation.
- [x] Wire connection setup into Control and Egress without starting full request flow.
- [x] Run `go test ./internal/natsx`.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/natsx`
- `make check`

## Acceptance Criteria

- Subject helpers generate exact-session subjects only.
- Invalid subject tokens are rejected.
- Max payload validation fails before runtime dispatch.
- Tests cover the NATS subject rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- List every subject format implemented.
- Note any subject intentionally deferred to a later task.

## Stop Conditions

- Stop if a helper would allow pool queue dispatch.
- Stop before adding JetStream or durable stream behavior.

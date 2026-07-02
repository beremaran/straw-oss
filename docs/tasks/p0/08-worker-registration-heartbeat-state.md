# 08 - Worker Registration, Heartbeat, and State

Status: not started

## Objective

Implement worker registration, heartbeat processing, worker state, duplicate-session handling, admin disable/drain, and cooldown.

## Required Planning Docs

- `docs/planning/05-component-boundaries.md`
- `docs/planning/11-worker-discovery-and-health.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 03 completed.
- Task 05 completed.
- Task 07 completed for worker credential validation.

## Out of Scope

- Do not implement route selection.
- Do not implement outbound HTTP execution.
- Do not store worker runtime state durably in Postgres except configured worker records.

## Expected Files

- Create or modify: `internal/control`
- Create or modify: `internal/egress`
- Create or modify: `internal/natsx`
- Test: registration, heartbeat, and worker state tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement registration validation for credentials, pool scope, version compatibility, and duplicate session replacement.
- [ ] Implement heartbeat states: ready, degraded, unhealthy, unavailable after 15s, dead after 30s.
- [ ] Store runtime state in Redis or in-process as specified, with TTL for Redis keys.
- [ ] Implement global disable, tenant disable, draining exclusion, and cooldown.
- [ ] Add tests for valid and invalid registration, duplicate sessions, stale sessions, health transitions, disable precedence, tenant isolation, draining, and cooldown.
- [ ] Run focused worker state tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused worker registration and state tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Worker state transitions match the timing policy.
- Duplicate and stale sessions are handled deterministically.
- Admin state respects global and tenant precedence.
- Tests cover the registration, heartbeat, and worker state rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document state names and timeout constants.
- Note how test time is controlled.

## Stop Conditions

- Stop if worker state would become durable control-plane config.
- Stop before adding routing decisions.

# 09 - Routing Evaluation

Status: not started

## Objective

Implement tenant-isolated routing snapshot evaluation and worker eligibility for P0.

## Required Planning Docs

- `docs/planning/10-routing-model.md`
- `docs/planning/11-worker-discovery-and-health.md`
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/27-security-controls.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 05 completed.
- Task 08 completed.

## Out of Scope

- Do not implement provider adapters.
- Do not implement automatic retries or replay workflows.
- Do not implement P1/P2 degraded policy beyond the P0 rules.

## Expected Files

- Create or modify: `internal/control`
- Test: routing tests under the Control package boundary.

## Steps

- [ ] Read all required planning docs.
- [ ] Evaluate routes by tenant and priority.
- [ ] Apply hard client hints according to the routing model.
- [ ] Filter workers by tenant, pool, health, draining, disable state, cooldown, and destination policy eligibility.
- [ ] Implement degraded-worker policy and no-match behavior.
- [ ] Implement sticky session success and failure behavior using the canonical Redis key structure in Section 10 (`straw:sticky:<tenant_id>:<sticky_session_id>`, tenant-scoped, TTL from the matched rule, re-pinned on permitted fallback).
- [ ] Add tests for priority order, tenant isolation, client hints, degraded policy, no match, sticky success, and sticky failure.
- [ ] Run focused routing tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused routing tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Routing never crosses tenant boundaries.
- Disabled, draining, unavailable, and cooldown workers are excluded.
- Tests cover the routing rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document tie-breaking behavior.
- Note any routing inputs intentionally ignored for P0.

## Stop Conditions

- Stop before adding automatic fallback execution.
- Stop if sticky behavior beyond the Section 10 key structure is ambiguous in planning docs.

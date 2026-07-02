# 13 - Rate Limits, Quotas, and Redis

Status: not started

## Objective

Implement Redis-backed P0 rate limits, quota hot counters, worker state storage, sticky sessions, and explicit Redis failure policies.

## Required Planning Docs

- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/30-testing-matrix.md`
- `docs/planning/33-risks.md`

## Prerequisites

- Task 05 completed.
- Task 08 completed.
- Task 09 completed.

## Out of Scope

- Do not claim billing-grade quota accuracy.
- Do not store durable config in Redis.
- Do not create Redis keys without TTL.

## Expected Files

- Create or modify: `internal/control`
- Create or modify: Redis helper package if already established.
- Test: rate limit, quota, Redis failure, and sticky session tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement rate limit dimensions and 429 `retry_after` behavior.
- [ ] Implement quota hot counters for operational admission control.
- [ ] Implement memory guardrail fallback.
- [ ] Store worker runtime state and sticky sessions with TTL.
- [ ] Implement explicit Redis loss behavior for rate limits, quotas, worker state, and sticky sessions.
- [ ] Add tests for dimensions, 429, retry_after, Redis fail policy, memory guardrail, quota request count, bandwidth accounting, sticky behavior, and non-billing-grade limits.
- [ ] Run focused Redis/admission tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused rate limit/quota tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Redis is ephemeral only and every key has a TTL.
- Rate limit and quota admission behavior is explicit during Redis outage.
- Quota behavior is operational and does not claim billing-grade accuracy.
- Tests cover the rate limit, quota, and Redis rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document Redis key prefixes and TTLs.
- State fail-open or fail-closed behavior per feature.

## Stop Conditions

- Stop before adding billing reconciliation.
- Stop if Redis would become required to reconstruct durable config.

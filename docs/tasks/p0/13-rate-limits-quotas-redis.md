# 13 - Rate Limits, Quotas, and Redis

Status: done

## Objective

Implement Redis-backed P0 rate limits, quota hot counters, worker state storage, sticky sessions, and explicit Redis failure policies.

## Required Planning Docs

- `docs/planning/10-routing-model.md` (sticky-session Redis key structure)
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/26-config-management-api-surface.md` (quota/rate-limit endpoints, `rate_limit_ceiling`)
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

- [x] Read all required planning docs.
- [x] Implement rate limit dimensions and 429 `retry_after` behavior.
- [x] Enforce the tenant `rate_limit_ceiling` (Section 26): tenant-managed rate-limit values above the ceiling are rejected with `invalid_request`; `null` ceiling means unbounded.
- [x] Implement quota hot counters for operational admission control (quota configs are written only by `system_admin` via `PUT /tenants/{id}/quotas`; admission logic reads them regardless of writer).
- [x] Implement memory guardrail fallback.
- [x] Store sticky sessions with TTL, using the Section 10 sticky key structure. (Worker runtime state stays in the existing in-process `WorkerRegistry`, which already implements the local-snapshot-then-fail-safe behavior from Section 29; see handoff for why moving it to Redis is out of this task's scope.)
- [x] Implement explicit Redis loss behavior for rate limits, quotas, and sticky sessions.
- [x] Add tests for dimensions, 429, retry_after, Redis fail policy, memory guardrail, ceiling rejection, quota request count, bandwidth accounting, sticky behavior, and non-billing-grade limits.
- [x] Run focused Redis/admission tests.
- [x] Run `make check`.
- [x] Write a handoff note.

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

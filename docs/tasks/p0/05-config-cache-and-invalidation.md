# 05 - Config Cache and Invalidation

Status: not started

## Objective

Implement Control's config snapshot cache with Postgres versioning and Redis invalidation.

## Required Planning Docs

- `docs/planning/21-state-and-storage.md`
- `docs/planning/24-static-configuration.md`
- `docs/planning/25-dynamic-configuration.md`
- `docs/planning/26-config-management-api-surface.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 04 completed.
- Postgres schema includes config versions.

## Out of Scope

- Do not implement the full config management REST API.
- Do not let Egress query Postgres or Redis.
- Do not cache data in Redis without TTL.

## Expected Files

- Create or modify: `internal/control`
- Create or modify: `internal/config`
- Test: config cache tests under the chosen package.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the immutable config snapshot shape consumed by routing and admission.
- [ ] Load snapshots from Postgres using version checks.
- [ ] Cache snapshots in-process in Control.
- [ ] Listen for Redis pub/sub invalidation.
- [ ] Correct missed invalidations with version checks.
- [ ] Add tests for cache hit, version conflict, invalidation, missed pub/sub recovery, and API key revocation invalidation.
- [ ] Run focused config cache tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused package tests for the config cache.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Postgres remains the source of truth.
- Redis invalidation only improves freshness and is not durable state.
- Missed invalidations are corrected by version checks.
- Tests cover the invalidation rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document cache lifetime and invalidation behavior.
- Note Redis outage behavior.

## Stop Conditions

- Stop if cache behavior would make Redis durable source of truth.
- Stop before adding unrelated config API endpoints.

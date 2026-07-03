# 21 - Redis Wiring and Config Invalidation

Status: not started

## Objective

Wire the existing Redis-backed runtime components into Control, implement Redis-backed config invalidation, and add the
durable fallback checks required when pub/sub messages are missed.

## Required Planning Docs

- `docs/planning/25-dynamic-configuration.md`
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/29-operational-behavior.md`

## Prerequisites

- Task 13 completed.
- Task 19 completed.

## Out of Scope

- Do not move `WorkerRegistry` runtime state to Redis in P0; the single-Control limitation is documented by the task 13
  handoff.
- Do not call admission checks from the request path (task 24).
- Do not use Redis as a durable config source.

## Expected Files

- Create or modify: `internal/control` invalidation publisher/subscriber wiring.
- Create or modify: `internal/redisx` if needed.
- Modify: `cmd/control/main.go`
- Modify: `go.mod`
- Test: focused Redis wiring and invalidation tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Promote the existing `github.com/redis/go-redis/v9` usage to a direct module dependency if it is still indirect.
- [ ] Dial Redis from `cmd/control/main.go` using `control.database.redis.*` configuration and `STRAW_REDIS_URL`.
- [ ] Construct the existing Redis-backed `RateLimiter`, `RateLimitAdmission`, `QuotaAdmission`, and `RedisStickyStore`
      at runtime so task 24 can consume them.
- [ ] Implement a Redis pub/sub `InvalidationPublisher` that publishes `straw:config:invalidate:<tenant_id>` messages
      after committed config writes.
- [ ] Implement a subscriber wired to `ConfigCache` invalidation, storing the latest seen tenant config version.
- [ ] Implement the durable fallback mechanism from Section 25: periodic Postgres tenant-version polling, plus forced
      Postgres version checks for sensitive operations such as API key, worker credential, and deny-rule changes.
- [ ] Enforce explicit Redis outage behavior for rate limits, quotas, sticky sessions, and invalidation according to
      configured fail policies.
- [ ] Add tests for Redis dial/config validation, pub/sub invalidation, missed-message version polling, sensitive forced
      checks, fail-open/fail-closed admission behavior, quota loss behavior, and sticky-session degradation.
- [ ] Run focused Redis wiring and invalidation tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused Redis wiring tests.
- Focused config invalidation tests.
- `go test ./internal/control ./internal/redisx ./cmd/...`
- `make check`

## Acceptance Criteria

- Control runtime constructs Redis-backed admission, quota, and sticky-session components instead of leaving them
  unwired.
- Config invalidation uses Redis pub/sub and at least one durable Postgres-backed version check.
- Redis data loss cannot corrupt durable config.
- Redis outage behavior is explicit for every runtime feature using Redis.

## Handoff Notes

- Document Redis key prefixes, TTLs, pub/sub channel shape, polling cadence, and fail policies.
- Note that request-path admission and sticky consumption are deferred to `docs/tasks/p0/24-control-request-dispatch-pipeline.md`.

## Stop Conditions

- Stop before storing durable config only in Redis.
- Stop if a Redis key would have no TTL or documented lifecycle.
- Stop if a deferral would have no owning task file.

# Handoff

Task: `docs/tasks/p0/13-rate-limits-quotas-redis.md`

## Summary

Implemented Redis-backed P0 rate limits (sliding-window log per dimension, with memory
guardrails), quota hot counters (fixed-window request-count and bandwidth), a Redis-backed sticky
session store, and explicit Redis failure policies for each, per `docs/planning/20` and
`docs/planning/21`. Added `github.com/redis/go-redis/v9` as the project's first live infra client
dependency — flagged and confirmed with the user before adding, since every other P0 subsystem
(NATS, Postgres) is still interface-plus-in-memory-fake only. `cmd/control/main.go` was not changed
to dial live Redis: the request pipeline (`RequestHandler`) does not yet call routing, admission, or
egress (that wiring is a later integration task per `internal/control/handler.go`'s existing
`// validated is used by later tasks` stub), so there is no call site yet to hand a live client to.
The new config-surface endpoints (`GET`/`PUT /rate-limits`) use the existing in-memory-config-store
pattern (matching `QuotaStore`) and are wired into the mux.

## Changed

- `internal/redisx/` (new package) — `Config` + `NewClient`, the shared Redis connection helper
  (`docs/planning/21`: Redis is ephemeral runtime state only).
- `internal/control/ratelimit.go` (new)
  - `RateLimitRule`/`RateLimitConfig`/`RateLimitConfigStore` (+ `InMemoryRateLimitConfigStore`):
    durable per-tenant rate-limit config with optimistic concurrency, matching the `QuotaStore`
    pattern.
  - `RateLimitCeiling` and `(RateLimitCeiling).exceeds`: cross-multiplied rate comparison so
    `Put` rejects tenant-managed values above the tenant's `system_admin`-set ceiling with
    `ErrRateLimitCeilingExceeded` (`docs/planning/26`); `nil` ceiling is unbounded.
  - `RateLimiter`: Redis sliding-window log via a Lua script (`ZADD`/`ZREMRANGEBYSCORE` on a
    per-dimension-key sorted set), with the two memory guardrails from `docs/planning/20`:
    - **Max entries per key** (default 10,000): once exceeded, the key is compacted and a guard
      marker denies all requests for that key until the window elapses (the doc's "switches to a
      conservative deny policy" — P0 does not implement full fixed-window-counter compaction, just
      the documented deny fallback).
    - **Max keys per tenant** (default 1,000): `WithinKeyBudget` tracks distinct non-tenant
      dimension keys in a Redis set; once exceeded, new dimension keys fall back to the
      tenant-level rule (`RateLimitAdmission.Check` applies this automatically).
  - `RateLimitAdmission`: evaluates every dimension rule in a tenant's config for one request and
    returns the most restrictive breach.
- `internal/control/quota_admission.go` (new)
  - `QuotaAdmission`: Redis fixed-window counters (`straw:quota:<tenant>:<YYYYMM>:requests` /
    `:bandwidth`), TTL to next-month boundary + 1 day grace.
  - `CheckAdmission`: atomic Lua script checks bandwidth-already-exhausted, then request-count,
    then conditionally increments (immediately for `count_on_admission`, the P0 default; deferred
    to `RecordSuccess` for `count_on_success`) — matches `docs/planning/20` "Request vs Attempt
    Semantics".
  - `AddBandwidth`: accumulates bytes actually transferred per attempt (both fallback attempts
    count, per doc).
  - `Usage`: read-only snapshot for `GET /quotas` display — explicitly not a reconciled/billing-grade
    total (`TestQuotaAdmissionNotBillingGrade` proves a lost Redis key silently resets usage rather
    than being repaired).
- `internal/control/sticky_redis.go` (new)
  - `StickyBackend` interface (`Get`/`Set`/`Refresh`) satisfied by both the existing in-process
    `StickyStore` and the new `RedisStickyStore`.
  - `RedisStickyStore`: same `straw:sticky:<tenant_id>:<sticky_session_id>` key shape as the
    in-process store. Any Redis error degrades `Get` to "no pin found" (never propagates an error),
    matching the documented sticky fail policy ("degrade according to route policy... may fail
    sticky requests") without special-casing outages in the router.
- `internal/control/routing.go` — `Router.sticky` field and `NewRouter`'s `sticky` parameter
  changed from `*StickyStore` to the `StickyBackend` interface so `RedisStickyStore` is a drop-in
  swap; no behavioral change for existing callers passing `*StickyStore`.
- `internal/control/tenant_store.go` — added `Tenant.RateLimitCeiling *RateLimitCeiling`
  (`docs/planning/26`; settable only by `system_admin`, `nil` = unbounded). No `PUT /tenants/{id}`
  endpoint exists yet to set this over HTTP (out of this task's scope — the full tenant resource
  schema is still task-06-minimal per `tenant_store.go`'s existing doc comment); tests set it
  directly via `TenantStore.Create`.
- `internal/control/errors.go` — added `ErrorResponse.RetryAfterMs` (`json:"retry_after_ms,omitempty"`)
  and `ErrorResponseFromCodeWithRetry`, per `docs/planning/14`/`20` ("Breaches return
  rate_limit_exceeded... and retry_after_ms when computable"; omitted when zero). This was flagged
  as remaining work in the task-12 handoff.
- `internal/control/admin_handlers.go` — `AdminHandlers.RateLimits RateLimitConfigStore` field;
  `GetRateLimits` (`GET /rate-limits`, roles `tenant_admin`/`operator`/`viewer`) and `PutRateLimits`
  (`PUT /rate-limits`, role `tenant_admin`, ceiling-validated, audited) handlers, matching the
  `GetQuotas`/`PutTenantQuotas` pattern.
- `cmd/control/main.go` — wired `control.NewInMemoryRateLimitConfigStore()` into `AdminHandlers` and
  registered the two new mux routes.
- `go.mod`/`go.sum` — added `github.com/redis/go-redis/v9` (user-approved; see Summary).
- Tests (new): `internal/control/redis_test_helper_test.go`, `ratelimit_test.go`,
  `quota_admission_test.go`, `sticky_redis_test.go`, plus additions to `admin_handlers_test.go`.

## Redis Key Prefixes and TTLs

| Prefix                                          | Purpose                          | TTL                                                    |
|--------------------------------------------------|-----------------------------------|--------------------------------------------------------|
| `straw:ratelimit:<tenant>:<dim>:<key>`            | sliding-window sorted set         | `window_ms + 1000`, refreshed on every admitted request |
| `straw:ratelimit:<tenant>:<dim>:<key>:guard`      | memory-guardrail deny marker      | `window_ms` (P0 conservative deny until window elapses) |
| `straw:ratelimit:keys:<tenant>`                   | per-tenant tracked dimension-key set | 24h, refreshed on touch                              |
| `straw:quota:<tenant>:<YYYYMM>:requests`          | monthly request-count counter     | seconds to next month boundary + 86,400s grace          |
| `straw:quota:<tenant>:<YYYYMM>:bandwidth`         | monthly bandwidth-byte counter    | seconds to next month boundary + 86,400s grace          |
| `straw:sticky:<tenant_id>:<sticky_session_id>`    | sticky pin (worker_id)            | `sticky_session_ttl_seconds` from the matched rule, refreshed on use |

Every key above is written with an explicit TTL; none are permanent.

## Fail Policy (Redis Outage)

| Feature          | Policy                                                                                          |
|-------------------|--------------------------------------------------------------------------------------------------|
| Rate limits       | Per-rule `fail_policy` (`open`/`closed`), configured per dimension rule. `RateLimitDecision.RedisFailure=true` marks a policy-driven (not counted) decision. |
| Quotas            | Per-tenant `QuotaConfig.RedisFailPolicy` (`open`/`closed`). Same `RedisFailure` marker.           |
| Sticky sessions    | Always degrades: `Get` reports "no pin" on any Redis error; `Set`/`Refresh` best-effort no-op. The router's existing `allow_sticky_fallback` logic then decides fail vs. re-pin — this is the "may fail sticky requests" outcome from `docs/planning/20`, achieved without new router logic. |
| Rate-limit key-budget check | Fails open (best-effort only; the per-key window check below it still applies its own configured policy). |

## Explicit P0 boundaries (see "Out of Scope" / "Stop Conditions" in the task file)

- No billing-grade quota reconciliation: `TestQuotaAdmissionNotBillingGrade` proves a lost Redis
  counter silently resets rather than being repaired from ClickHouse.
- Worker runtime state (heartbeat/load) was **not** moved into Redis. `WorkerRegistry`
  (`internal/control/worker_registry.go`, task 08) remains the P0 in-process store; its existing
  availability/dead timeouts already implement doc 29's "use local snapshot for short TTL, then
  fail safe" for worker availability. Moving worker session/heartbeat/load and cooldown state to Redis is now owned
  by `docs/tasks/p0/45-redis-backed-worker-runtime-state.md`.
- Redis eviction-priority guidance from `docs/planning/21` ("quota/rate counters and worker
  availability must not be evicted before best-effort cache data") is a Redis server/`redis.conf`
  concern (logical DBs, `maxmemory-policy` per key prefix), not something enforced in Go code; not
  implemented here.

## Verification

```sh
make check
```

Result: PASS. `go test ./...` all green; `golangci-lint run --max-issues-per-linter 0
--max-same-issues 0` reports `0 issues`.

Focused run:

```sh
go test ./internal/control/... -run 'RateLimit|Quota|Sticky|Router|ErrorResponseFromCodeWithRetry' -v
```

Result: PASS (against a local Redis on `127.0.0.1:6379`, e.g. `docker run -d -p 6379:6379
redis:7-alpine`, matching `docker-compose.yml`). Redis-dependent tests call `t.Skip` when Redis is
unreachable — verified by stopping the container mid-run — so `make check` does not fail in an
environment without Redis; only the Redis-backed behavior goes unverified in that case.

## Reviewer Start Points

- `internal/control/ratelimit.go` — dimensions, ceiling enforcement, sliding-window Lua script,
  guardrails.
- `internal/control/quota_admission.go` — fixed-window Lua scripts, request/bandwidth semantics.
- `internal/control/sticky_redis.go` + `internal/control/routing.go` (`StickyBackend`) — Redis
  sticky swap-in.
- `internal/control/admin_handlers.go` (`GetRateLimits`/`PutRateLimits`) and
  `cmd/control/main.go` (mux wiring).

## Remaining Work

- Full request-path wiring: `RequestHandler` (`internal/control/handler.go`) does not yet call
  `Router.Evaluate`, `RateLimitAdmission.Check`, or `QuotaAdmission.CheckAdmission` — routing
  itself isn't wired into the HTTP path yet either (pre-existing `// validated is used by later
  tasks` stub). When that integration task lands, it should also dial a live Redis client via
  `internal/redisx.NewClient` in `cmd/control/main.go` and pass it to `RateLimiter`,
  `QuotaAdmission`, and `RedisStickyStore`, and use `ErrorResponseFromCodeWithRetry` to surface
  429s with `retry_after_ms`.
- No `PUT /tenants/{id}` endpoint exists yet to set `rate_limit_ceiling` over HTTP; add it when the
  full tenant resource schema task is picked up.

> RESOLVED 2026-07-05 (P0 audit): both items closed by task 24 (done). The live dispatch pipeline now calls
> `RateLimitAdmission.Check` and `QuotaAdmission.CheckAdmission` in `admit()` (`internal/control/dispatcher.go:254`/`267`)
> and `Router.Evaluate` in `route()` (`internal/control/dispatcher.go:290`). `PUT /tenants/{id}` exists as
> `AdminHandlers.UpdateTenant` and sets `rate_limit_ceiling` — see `internal/control/admin_handlers.go:279` (ceiling
> parsed at `:298`).

## Blockers

- None.

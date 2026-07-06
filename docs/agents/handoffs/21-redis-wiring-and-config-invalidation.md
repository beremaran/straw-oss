# Handoff

Task: `docs/tasks/p0/21-redis-wiring-and-config-invalidation.md`

## Changed

- `internal/config/config.go` — New `RedisConfig` (`url_env`, `dial_timeout_ms`,
  `read_timeout_ms`, `write_timeout_ms`) under `DatabaseConfig.Redis`, defaults
  `url_env=STRAW_REDIS_URL`, `2000/500/500` ms timeouts (mirrors `PostgresConfig`'s
  `dsn_env`/`STRAW_POSTGRES_DSN` pattern).
- `internal/redisx/redisx.go` — New `ResolveURL(urlEnv string) (string, error)` (reads the
  named env var, defaulting to `STRAW_REDIS_URL`, erroring if unset/empty) and
  `NewClientFromURL(rawURL string, cfg Config) (*redis.Client, error)` (parses a `redis://`/
  `rediss://` URL via `redis.ParseURL`, then overrides dial/read/write timeouts from `cfg`).
- `internal/control/invalidation_redis.go` — New. `RedisInvalidationPublisher` (implements the
  existing `InvalidationPublisher` interface: `PUBLISH straw:config:invalidate:<tenant_id>
  <version>`) and `RedisInvalidationSubscriber` (`PSUBSCRIBE straw:config:invalidate:*`, applies
  each message to a `*ConfigCache` via the existing `ApplyInvalidation`). `Run(ctx)` blocks on the
  subscribe ack (`pubsub.Receive`) before consuming `pubsub.Channel()`, so callers have a clean
  synchronization point and malformed payloads are logged and skipped rather than crashing the
  loop.
- `internal/control/config_cache.go` — New `ConfigCache.PollAllTenants(ctx)`: the durable
  Postgres-backed fallback for a missed pub/sub message (docs/planning/25 "periodic Postgres
  version poll"). It re-checks `SyncTenantVersion` for every tenant currently in the cache;
  uncached tenants don't need it since they load fresh on their next `Snapshot` call regardless.
- `internal/control/admin_handlers.go` — `AdminHandlers` gained `RateLimiter`,
  `RateLimitAdmission`, `QuotaAdmission`, `StickySessions` fields. Nothing in this task reads
  them; they exist so `cmd/control/main.go` can construct the real Redis-backed components and
  hand them to the eventual request dispatch pipeline (docs/tasks/p0/24) instead of that task
  having to build its own.
- `cmd/control/main.go`:
  - `openRedis` resolves `STRAW_REDIS_URL` (via `redisCfg.URLEnv`) and dials. An unresolvable/
    malformed URL fails Control startup like a missing Postgres DSN does; an unreachable-but-
    configured Redis only logs a warning and returns the client anyway (docs/planning/29 "Redis
    unavailable: Apply configured fail policy" — every Redis-backed component below already
    applies its own fail policy per call, so Control must still serve).
  - `wireConfigInvalidation` builds the real `ConfigCache` (Postgres store + `
    RedisInvalidationPublisher`) and starts `runInvalidationSubscriber` (reconnect-on-error loop
    around `RedisInvalidationSubscriber.Run`) and `runInvalidationPoller` (30s ticker calling
    `PollAllTenants`) as goroutines tied to the process shutdown context.
  - `buildAdminHandlers` now also constructs `RateLimiter`/`RateLimitAdmission`/`QuotaAdmission`/
    `RedisStickyStore` against the live Redis client and sets them on `AdminHandlers`.
  - `runControl` now creates the shutdown `signal.NotifyContext` itself and threads it through
    to `serveControlHTTP` (previously created inside `serveControlHTTP`), so the invalidation
    subscriber/poller goroutines and the HTTP server share one shutdown signal. Extracted
    `setupAPIKeyStore` to keep `runControl` under the `funlen` limit.
- Tests:
  - `internal/redisx/redisx_test.go` — `ResolveURL` (missing env, default env name, named env),
    `NewClientFromURL` (timeout override, invalid URL error).
  - `internal/control/invalidation_redis_test.go` — real-Redis pub/sub round trip (publish on one
    client, subscriber applies the invalidation to a separately-constructed `ConfigCache`, proven
    by polling `Snapshot` until the version advances or a 2s deadline), plus a publish-against-
    unreachable-Redis error case.
  - `internal/control/config_cache_test.go` — `TestConfigCachePollAllTenantsRecoversMissedInvalidation`:
    two cached tenants, one's durable version silently advances (no `ApplyInvalidation` call),
    `PollAllTenants` refreshes only that one.
  - `internal/config/config_test.go` — `TestLoadControlRedisDefaults` confirms `RedisConfig`
    defaults apply when `database.redis` is omitted from the config file.
  - `cmd/control/main_test.go` — `openRedis`: missing URL env fails, malformed URL fails,
    unreachable-but-configured URL still returns a usable (if unreachable) client.

## Verification

```sh
go test ./internal/control ./internal/redisx ./cmd/...
make check
```

Result: **pass**. Redis was started locally (`docker compose up -d redis`, `redis:7-alpine` on
`127.0.0.1:6379`, matching `internal/control/redis_test_helper_test.go`'s expected address) so
every Redis-backed test in `internal/control` ran for real rather than skipping.
`golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reports **0 issues** (fixed a
`contextcheck` hit by using `context.WithoutCancel(ctx)` for the shutdown timeout, three
`noinlineerr` hits, a `wsl_v5` blank-line hit, and split `runControl` to satisfy `funlen`).

## Reviewer Start Points

- `internal/control/invalidation_redis.go` — publisher/subscriber.
- `internal/control/config_cache.go` — `PollAllTenants`.
- `cmd/control/main.go` — `openRedis`, `wireConfigInvalidation`, `buildAdminHandlers`.
- `internal/redisx/redisx.go` — `ResolveURL`, `NewClientFromURL`.

## Remaining Work

- Request-path admission (rate limits, quotas, sticky-session consumption) and the constructed
  `RateLimiter`/`RateLimitAdmission`/`QuotaAdmission`/`StickySessions` sitting unused on
  `AdminHandlers` are `docs/tasks/p0/24-control-request-dispatch-pipeline.md`'s scope, as called
  out in the task's own Out of Scope section.
- `WorkerRegistry` runtime state stayed single-Control/in-memory in this task's scope. The Redis-backed worker
  session/heartbeat/load and cooldown state required by `docs/planning/21` is now owned by
  `docs/tasks/p0/45-redis-backed-worker-runtime-state.md`.
- Grepped the diff for `InMemory|stub|fake|synthetic|TODO`: no hits in the changed production
  files. (Tests use the pre-existing `fakeSnapshotStore`/`fakeInvalidationPublisher` doubles from
  task 19/20's `config_cache_test.go`, unchanged here.)

## Blockers

- None.

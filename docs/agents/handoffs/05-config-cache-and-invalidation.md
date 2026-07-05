# Handoff

Task: `docs/tasks/p0/05-config-cache-and-invalidation.md`

## Changed

- Added `internal/config.TenantSnapshot` as the immutable tenant config snapshot container with deep-copy helpers.
- Added `internal/control.ConfigCache` with versioned snapshot reads, write-through saves, Redis invalidation handling, and Postgres version polling.
- Added focused tests for cache hits, version conflicts, invalidation-driven reloads, missed pub/sub recovery, and API key revocation invalidation.

## Verification

```sh
go test ./internal/control ./internal/config
make check
```

Result:

- Passed.

## Reviewer Start Points

- [internal/control/config_cache.go](/Users/beremaran/projects/straw/internal/control/config_cache.go)
- [internal/control/config_cache_test.go](/Users/beremaran/projects/straw/internal/control/config_cache_test.go)

## Remaining Work

- (Corrected by audit 2026-07-03.) At runtime the cache is backed by `NewInMemorySnapshotStore` and the
  `InvalidationPublisher` is `nil` (`cmd/control/main.go`). No Postgres-backed `SnapshotStore` and no Redis
  pub/sub invalidation implementation exist. Owned by `docs/tasks/p0/19-postgres-config-stores-and-snapshot.md`
  and `docs/tasks/p0/21-redis-wiring-and-config-invalidation.md`.

  > RESOLVED 2026-07-05 (P0 audit): closed by tasks 19 and 21 (both done). Real Redis publisher wired at
  > `cmd/control/main.go:280` (`control.NewRedisInvalidationPublisher`, defined `internal/control/invalidation_redis.go:30`);
  > Postgres `SnapshotStore` delivered by `internal/control/postgres_config_store.go:106` and wired via
  > `control.NewPostgresConfigStore(pool)` at `cmd/control/main.go:125`/`280`.

## Blockers

- None.

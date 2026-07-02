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

- None.

## Blockers

- None.

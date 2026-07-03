# Handoff

Task: `docs/tasks/p0/19-postgres-config-stores-and-snapshot.md`

## Changed

- `internal/config/snapshot.go` — Extended `TenantSnapshot` to carry the captured
  `ConfigVersion` plus config-layer carrier types: `RoutingRule`/`MatchConditions`,
  `ExecutorPool`, `DenyRule`, `InjectionPolicy`/`InjectionOperation`, `FingerprintProfile`,
  `RateLimitRule`, `QuotaConfig`, `WorkerAdminState`, `TenantWorkerOverride`, alongside the
  existing `RevokedAPIKeyIDs`. `Clone()` now deep-copies every mutable slice (incl. nested
  `Match.Tags` and injection `Operations`) so cached snapshots are immutable. These are
  config-layer data carriers, deliberately separate from the control-package runtime types
  (`control.RoutingRule`, ...) to keep `internal/config` free of a `control` import; tasks 22/24
  map them into runtime types when they consume the snapshot.
- `migrations/postgres/0002_config_soft_delete_and_fingerprints.sql` — New idempotent migration:
  adds `deleted_at` soft-delete columns to `routing_rules`, `executor_pools`, `deny_rules`,
  `injection_policies`; seeds built-in **global** `fingerprint_profiles` (`default`, `chrome_120`,
  `firefox_121`, `safari_17`) via `INSERT ... WHERE NOT EXISTS`. Re-applies cleanly (verified by
  `scripts/check-postgres-migrations.sh`, which runs the full set twice).
- `internal/control/postgres_config_store.go` — New `PostgresConfigStore` (holds the pool).
  `writeTenantConfig` runs each resource write + `tenant_config_versions` increment +
  redacted `config_audit_source` append in **one transaction**. Write methods:
  `UpsertRoutingRule`/`DeleteRoutingRule`, `UpsertExecutorPool`/`DeleteExecutorPool`,
  `UpsertDenyRule`/`DeleteDenyRule`, `UpsertInjectionPolicy`/`DeleteInjectionPolicy` (deletes are
  soft, via fixed per-table statements — no dynamic SQL). Upserts key on the stable
  `(tenant_id, id)` for idempotent stable IDs. Injection audit redacts each op's `value_base64`
  to `[redacted]` (the snapshot keeps the real value for request-time injection). Durable worker
  admin: `SetGlobalWorkerAdminConfig` / `SetTenantWorkerOverrideConfig` (audit-in-tx) and
  `ListWorkerAdminStates` / `ListTenantWorkerOverrides` for startup rehydration.
- `internal/control/postgres_snapshot_store.go` — Makes `PostgresConfigStore` implement
  `SnapshotStore`. `CurrentTenantConfigVersion`, `LoadTenantSnapshot` (assembles the full snapshot
  from all config tables inside one read-only `RepeatableRead` transaction; rejects a stale
  requested version with `ErrVersionConflict`, matching `InMemorySnapshotStore` semantics),
  and `SaveTenantSnapshot` (optimistic version bump + re-assemble — used by the existing
  API-key/worker-credential revocation invalidation path). Deleted resources are excluded from
  the assembled snapshot.
- `internal/control/postgres_admission_config_store.go` — Postgres `QuotaStore`
  (`postgresQuotaStore`) and `RateLimitConfigStore` (`postgresRateLimitConfigStore`), plus the
  transactional `PutQuotaConfig` / `PutRateLimitConfig` on `PostgresConfigStore` (version bump +
  audit in one tx). Rate-limit `Put` validates each limit against the tenant `rate_limit_ceiling`.
- `internal/control/admin_handlers.go` — Added optional `ConfigWrites ConfigWriteStore` and
  `WorkerAdmin WorkerAdminStore` fields. When `ConfigWrites` is set (the binary), quota, rate-limit,
  and worker-admin writes go through the single-transaction (version + audit) path; when nil (unit
  tests) the pre-existing in-memory two-step path is used, so existing handler tests are unchanged.
- `internal/control/worker_handlers.go` — Worker disable/enable (global and tenant) now persist
  durably; drain/undrain stay runtime-only. Tenant disable/enable also bumps the tenant config
  version so snapshots invalidate.
- `cmd/control/main.go` — Constructs `PostgresConfigStore` and uses it as the `ConfigCache`
  `SnapshotStore`, `ConfigWrites`, and `WorkerAdmin`; wires Postgres quota/rate-limit stores;
  rehydrates durable worker disable state into the runtime registry at startup. No
  `NewInMemory*` store remains in the binary.
- `internal/config/snapshot_test.go`, `internal/control/postgres_config_store_test.go` — Tests
  (below). `internal/control/postgres_store_test.go` — the shared `newIdentityTestPool` now
  re-applies the idempotent migrations after its `TRUNCATE ... CASCADE` (which also empties
  `fingerprint_profiles`) so the seeded global profiles the binary always has are restored.

## Verification

```sh
make check
```

Result: **pass** — `fmt-check` clean, `go test ./...` pass (Postgres integration tests skip
without `STRAW_TEST_POSTGRES_DSN`), `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`
reports `0 issues`.

Postgres integration verified against a real DB (`docker compose up -d postgres`, migrations
applied, `STRAW_TEST_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/straw go test ./...`):

- `TestTenantSnapshotCloneDeepCopiesPolicySlices` — snapshot immutability (deep clone).
- `TestPostgresConfigStoreSnapshotAssembly` — full assembly of every resource from Postgres,
  version match, soft-deleted routing rule excluded, seeded fingerprint present, stale-version
  `ErrVersionConflict`.
- `TestPostgresConfigStoreRedactsInjectionPolicyAudit` — audit `new_value_json` redacts the
  secret `value_base64`.
- `TestPostgresConfigStoreWriteIsAtomic` — a write that fails inside the transaction leaves the
  tenant config version unchanged and writes no audit row.
- `TestPostgresSaveTenantSnapshotOptimisticVersioning` — optimistic version bump / conflict.
- `scripts/check-postgres-migrations.sh` — migration set idempotent (applied twice on a clean DB).

## Reviewer Start Points

- `internal/config/snapshot.go` — snapshot shape and `Clone()`.
- `internal/control/postgres_config_store.go` — `writeTenantConfig` transaction boundary.
- `internal/control/postgres_snapshot_store.go` — assembly + version handling.
- `cmd/control/main.go` — binary wiring (`configStore` used as snapshot store, config writes,
  worker admin) and startup rehydration.
- `migrations/postgres/0002_config_soft_delete_and_fingerprints.sql`.

## Remaining Work

- None faked or stubbed in the running binary: config snapshots and all mutable config writes
  (routing, executor pools, deny, injection, rate-limit, quota, worker admin) are Postgres-backed
  end to end. The `InMemory*` stores remain only as unit-test doubles.
- Deferred by design to owning tasks (not this task's scope):
  - Config-management HTTP APIs for routing/deny/injection/read-only fingerprint →
    `docs/tasks/p0/20-config-admin-apis.md` (calls the write methods added here).
  - Redis pub/sub invalidation + durable version polling → `docs/tasks/p0/21-redis-wiring-and-config-invalidation.md`
    (the `ConfigCache` invalidation publisher is wired as `nil` until then).
  - Request dispatch consuming the assembled snapshot → `docs/tasks/p0/24-control-request-dispatch-pipeline.md`.
- Not a deferral: durable worker admin is **disable-only** (`worker_admin_state` /
  `tenant_worker_admin_state` carry no drain column per Section 21); worker *drain* is
  runtime-only state on `WorkerRegistry`, so it is intentionally not persisted. This resolves the
  task step's "disable/drain" wording in favor of the planning doc (Section 21), which the board
  rules say wins on conflict.

## Blockers

- None.

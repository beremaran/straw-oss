# Handoff

Task: `docs/tasks/p0/30-executor-pool-config-api.md`

## Changed

- `migrations/postgres/0004_executor_pool_degraded_policy.sql`: adds
  `executor_pools.allow_degraded_workers boolean not null default false`, the
  storage backing the schema's `allow_degraded_workers` field, which previously
  had no column at all.
- `internal/config/snapshot.go`: `config.ExecutorPool` gains
  `AllowDegradedWorkers bool`.
- `internal/control/postgres_config_store.go`: added `ExecutorPoolRecord`
  (mirrors `RoutingRuleRecord`); `UpsertExecutorPool` now takes
  `expectedVersion`, runs the same optimistic-concurrency check as
  `UpsertRoutingRule`/`UpsertDenyRule` (`ErrConfigResourceVersionConflict` on
  mismatch), persists `allow_degraded_workers`, and returns
  `(ExecutorPoolRecord, tenantVersion, error)` instead of `(uint64, error)`.
  `DeleteExecutorPool` unchanged (the `executor_pool` soft-delete query already
  existed).
- `internal/control/postgres_config_list_store.go`: added `GetExecutorPool`
  and `ListExecutorPools` (same pagination/sort contract as the other config
  resources).
- `internal/control/postgres_snapshot_store.go`: `readExecutorPools` now also
  reads `allow_degraded_workers` into the snapshot.
- `internal/control/config_resource_store.go`: added the `ExecutorPoolStore`
  interface and `InMemoryExecutorPoolStore` test double (production never
  wires the in-memory store — see `cmd/control/main.go` below); added the
  `ExecutorPoolRecord` case to `currentVersionOf` (hoisted the `entry.deleted`
  check out of the type switch to keep `cyclop` under the complexity budget).
- `internal/control/admin_handlers.go`: added `ExecutorPools ExecutorPoolStore`
  field to `AdminHandlers`.
- `internal/control/config_admin_handlers.go`: added
  `ListExecutorPools`/`CreateExecutorPool`/`UpdateExecutorPool`/`DeleteExecutorPool`
  handlers, following the routing-rule pattern exactly (client-supplied stable
  ID, `tenant_admin`-only write / `tenant_admin`+`operator`+`viewer` read per
  `docs/planning/26`, `expected_config_version` → 409 with
  `details.current_config_version`, soft delete, config-version bump +
  invalidation publish + audit record on every write).
- `internal/control/dispatcher.go`: `route()` no longer calls
  `NewStaticPoolPolicyProvider(nil)`; it now builds the policy list from
  `snapshot.ExecutorPools` via a new `poolPoliciesFromSnapshot` helper, so
  degraded-pool policy is sourced from the tenant's live config instead of
  always defaulting every pool to `AllowDegradedWorkers=false`.
- `cmd/control/main.go`: wired `ExecutorPools: configStore` (the real
  `*PostgresConfigStore`, same as the other config resources) into
  `buildAdminHandlers`, and registered
  `GET/POST /api/v1/config/executor-pools` and
  `PUT/DELETE /api/v1/config/executor-pools/{id}` in
  `serveConfigResourceRoutes`.
- Tests: `internal/control/config_admin_handlers_test.go` (pool CRUD + RBAC,
  tenant isolation, pagination defaults — mirrors the routing-rule test set),
  `internal/control/dispatcher_test.go`
  (`TestDispatcherRoutePoolPolicyFromSnapshot`: a degraded-only candidate is
  rejected with no pool in the snapshot, then admitted once the snapshot's
  `ExecutorPools` entry has `allow_degraded_workers=true`),
  `internal/control/postgres_config_store_test.go` (extended the existing
  snapshot-assembly integration test to create a pool with
  `AllowDegradedWorkers: true` and assert it survives the Postgres round trip),
  `internal/control/admin_handlers_test.go` (wired `executorPools` into the
  shared `testAdmin` harness), `internal/control/postgres_config_store_test.go`
  (updated the pre-existing `UpsertExecutorPool` call site for the new
  signature).

## Verification

```sh
go test ./internal/control ./cmd/...
make check
STRAW_TEST_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/straw?sslmode=disable make check
./scripts/check-postgres-migrations.sh
```

Result: all pass. `make check` (fmt-check + `go test ./...` + golangci-lint
`--max-issues-per-linter 0 --max-same-issues 0`) is clean. Ran the Postgres
integration tests (`TestPostgresConfigStoreSnapshotAssembly`,
`TestPostgresConfigStoreRedactsInjectionPolicyAudit`,
`TestPostgresConfigStoreWriteIsAtomic`) against a live `postgres:16-alpine`
container already running locally (`straw-postgres-1`), not just the
in-memory test doubles — snapshot assembly now also asserts the new
`allow_degraded_workers` column round-trips. `check-postgres-migrations.sh`
confirms migration 0004 applies cleanly and is idempotent (reruns as a no-op
NOTICE).

## Reviewer Start Points

- `internal/control/config_admin_handlers.go` (`---- executor pools ----`
  section) — HTTP surface.
- `internal/control/postgres_config_store.go`
  (`UpsertExecutorPool`)/`postgres_config_list_store.go`
  (`GetExecutorPool`/`ListExecutorPools`) — storage.
- `internal/control/dispatcher.go` (`poolPoliciesFromSnapshot`, `route()`) —
  degraded-pool policy wiring; this landed in `dispatcher.go` rather than
  `main.go` because that is where `NewStaticPoolPolicyProvider` is actually
  constructed (per-request, from the tenant snapshot already in scope) — there
  was never a call site in `main.go` to change.

## Endpoints and roles (docs/planning/26)

| Method | Path                              | Roles                                 |
|--------|-----------------------------------|----------------------------------------|
| GET    | `/api/v1/config/executor-pools`   | `tenant_admin`, `operator`, `viewer`   |
| POST   | `/api/v1/config/executor-pools`   | `tenant_admin`                        |
| PUT    | `/api/v1/config/executor-pools/{id}` | `tenant_admin`                     |
| DELETE | `/api/v1/config/executor-pools/{id}` | `tenant_admin`                     |

Create/update use a client-supplied stable `id` (like routing rules) and
`expected_config_version` optimistic concurrency (409 `conflict` with
`details.current_config_version` on mismatch). Delete is soft delete
(`deleted_at`), excluding the pool from list/get and from future assembled
snapshots while preserving audit history.

## Degraded-policy default

A pool absent from the snapshot (or never created) defaults to
`AllowDegradedWorkers=false` via `StaticPoolPolicyProvider`'s zero-value
fallback — unchanged behavior from before this task, just now driven by real
config instead of an empty `nil` list every pool would have hit anyway.

## Remaining Work

- None for this task's acceptance criteria (all three are met and verified
  above).
- Pre-existing gap, *not* introduced or touched by this task: the
  `docs/planning/26` P0 Executor Pool schema also lists `allowed_ip_types`,
  `allowed_countries`, and `allowed_regions` fields. Neither `config.ExecutorPool`,
  the `executor_pools` table, nor any pool-eligibility code path
  (`WorkerRegistry.CandidatesForPool` / `PoolCandidate`) has ever implemented
  these — they are absent from the codebase entirely, not just from this
  task's HTTP surface. This task's Expected Files list did not include the
  routing/candidate-eligibility files that would need to change to consume
  them, so implementing them was out of this task's scope per its Steps and
  "Out of Scope" note (don't add pool-policy fields beyond what existing
  infrastructure consumes). No task file currently owns closing this gap —
  flagging it here rather than silently deferring it, per the Completion
  Audit's unowned-deferral rule. Recommend a new task if this should be
  closed. **[Update 2026-07-05: now owned by
  `docs/tasks/p0/42-executor-pool-capability-fields.md`.]**

## Blockers

- None.

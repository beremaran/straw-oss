# Handoff

Task: `docs/tasks/p0/42-executor-pool-capability-fields.md`

## Changed

- `migrations/postgres/0005_executor_pool_capabilities.sql` — adds `allowed_ip_types_jsonb`,
  `allowed_countries_jsonb`, `allowed_regions_jsonb` (jsonb, default `'[]'`, `IF NOT EXISTS`), matching the
  `tags_jsonb`/`0004` convention already used on this table.
- `internal/config/snapshot.go` — `ExecutorPool` gains `AllowedIPTypes`, `AllowedCountries`, `AllowedRegions`.
- `internal/control/postgres_config_store.go` — `UpsertExecutorPool` persists the three fields; extracted
  `marshalPoolCapabilityFields` to keep the function under the `funlen` lint limit.
- `internal/control/postgres_config_list_store.go` — `GetExecutorPool`/`ListExecutorPools` read the three fields;
  extracted `unmarshalPoolCapabilityFields` (shared with snapshot assembly).
- `internal/control/postgres_snapshot_store.go` — `readExecutorPools` reads the three fields into the snapshot,
  reusing `unmarshalPoolCapabilityFields`.
- `internal/control/config_admin_handlers.go` — `executorPoolRequest`/`executorPoolResponse` carry the three
  fields; `upsertExecutorPool` validates `allowed_ip_types` against the P0 taxonomy
  (`datacenter | residential | mobile | isp | unknown`, `docs/planning/10-routing-model.md:31`) and rejects
  unknown values with 400.
- `internal/control/routing.go` — `PoolPolicy` carries the three restriction lists; new `poolAllows` excludes a
  `PoolCandidate` whose claimed capabilities aren't a subset of a non-empty restriction, called from
  `eligibleCandidates`.
- `internal/control/dispatcher.go` — `poolPoliciesFromSnapshot` copies the three fields from `config.ExecutorPool`
  into `PoolPolicy`.
- Tests: `config_admin_handlers_test.go` (CRUD round-trip + invalid-ip-type rejection),
  `dispatcher_test.go` (`TestDispatcherRoutePoolCapabilityRestriction`, non-matching vs. matching candidate),
  `postgres_config_store_test.go` (live-Postgres snapshot round-trip). Also switched three pre-existing
  `"datacenter"` string literals (`ratelimit_test.go`, `worker_registry_test.go`) to the new `ipTypeDatacenter`
  const to clear a `goconst` lint trip caused by the new occurrences.

## Acceptance Criteria Verdicts

Independent verifier (fresh agent, given only the task file + diff) confirmed all four criteria pass; see its
per-criterion table below.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Pool CRUD accepts, persists, returns the three fields | VERIFIED | `config_admin_handlers.go` request/response structs + `upsertExecutorPool`; `postgres_config_store.go` `UpsertExecutorPool`; `postgres_config_list_store.go` `Get`/`ListExecutorPools` | `TestExecutorPoolCRUDAndRBAC`, `TestPostgresConfigStoreSnapshotAssembly` (live Postgres) |
| Empty restrictions preserve current behavior | VERIFIED | `routing.go` `poolAllows` short-circuits `true` when a restriction list is empty | Existing pool/dispatcher tests stay green (no restriction fields set) |
| Non-empty restriction excludes non-matching workers, matching worker eligible | VERIFIED | `routing.go` `eligibleCandidates` → `poolAllows`, reuses `subset()` (same helper as worker-credential capability scoping in `worker_registry.go`) | `TestDispatcherRoutePoolCapabilityRestriction` |
| No remaining unimplemented `docs/planning/26` P0 Executor Pool field | VERIFIED | `internal/config/snapshot.go` `ExecutorPool` now has every field from the planning doc's example object | n/a (structural) |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/26` Executor Pool `allowed_ip_types` | implemented | `config_admin_handlers.go`, `postgres_config_store.go`, `postgres_config_list_store.go`, `postgres_snapshot_store.go` |
| `docs/planning/26` Executor Pool `allowed_countries` | implemented | same files |
| `docs/planning/26` Executor Pool `allowed_regions` | implemented | same files |
| `docs/planning/10` pool eligibility / worker capability matching | implemented | `routing.go:poolAllows`, wired via `dispatcher.go:poolPoliciesFromSnapshot` |
| `docs/planning/10` `ip_type` value taxonomy (`datacenter|residential|mobile|isp|unknown`) | implemented | `config_admin_handlers.go:validIPTypes` |

No other fields from the cited planning-doc sections were touched by this task; all pre-existing pool fields
(`tags`, `allow_degraded_workers`, `executor_type`, `enabled`) were out of scope per the task's Out of Scope note
and are unchanged.

## Verification

```sh
make check
```

Result: clean (`gofmt`, `golangci-lint --max-issues-per-linter 0 --max-same-issues 0` → 0 issues, `go test ./...`
→ all pass).

- Postgres-backed tests: ran against the dedicated `straw_test` database (compose stack's Postgres container,
  never the live `straw` database):
  `STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...`
  — all pass, including `TestPostgresConfigStoreSnapshotAssembly`'s new capability-field round-trip assertion.
  Migration `0005` was applied to `straw_test` and re-applied a second time to confirm idempotency (`NOTICE:
  column ... already exists, skipping`).
- Live compose verification: skipped. The session's auto-mode permission guard blocked applying the migration to
  the live `straw` database and blocked rebuilding/restarting the `control` binary against it, citing this repo's
  own `AGENTS.md`/`deploy/docker/README.md` rule to never point work at the compose stack's live database. This
  task's change is a config-schema/routing-eligibility addition with no egress-request-path behavior change when
  a pool has no restrictions set (the existing default), so the risk of an unverified live gap is low, but a
  live pool-restriction CRUD + assignment check has not been driven end-to-end. If this needs live verification,
  it should be done as a deliberate, user-approved step (restart `control` after `docker compose` picks up the
  new migration via its embedded startup migrator, then create a restricted pool and confirm a non-matching
  worker is excluded).
  [Update 2026-07-06: the user approved that step; now owned by `docs/tasks/p0/46-live-compose-verification.md`.]

## Reviewer Start Points

- `internal/control/routing.go` (`poolAllows`, `eligibleCandidates`)
- `internal/control/config_admin_handlers.go` (`validIPTypes`, `upsertExecutorPool`)
- `internal/control/dispatcher_test.go` (`TestDispatcherRoutePoolCapabilityRestriction`)

## Remaining Work

- None. Nothing in this task's diff is faked, stubbed, or deferred.

## Blockers

- None functionally, but the work is currently uncommitted in the working tree (per this run's scope — commit is
  a separate, explicit user action).
  [Update 2026-07-06 sweep: stale — the work has since been committed; the working tree is clean.]

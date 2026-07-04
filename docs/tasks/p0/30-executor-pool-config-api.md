# 30 - Executor Pool Config API

Status: done

## Objective

Expose the P0 `/executor-pools` config-management CRUD surface over the existing Postgres pool store, and source pool
policy into routing instead of the current `nil` placeholder.

## Context (gap being closed)

The 2026-07-04 review found `docs/planning/26`'s P0 `/executor-pools` endpoints (POST/GET/PUT/DELETE) are not wired,
even though the Postgres store methods (`PostgresConfigStore.UpsertExecutorPool`, `DeleteExecutorPool`) and the
snapshot read (`readExecutorPools`) already exist from task 19. Separately, `dispatcher.go` constructs
`NewStaticPoolPolicyProvider(nil)`, so degraded-pool policy has no configuration source. This task adds the HTTP
surface and a list/get store method, and feeds pool policy from the snapshot.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (executor-pool endpoints, stable-ID rule, RBAC, soft-delete)
- `docs/planning/10-routing-model.md` (pool eligibility and degraded-pool policy)
- `docs/planning/21-state-and-storage.md`

## Prerequisites

- Task 19 completed (pool store + snapshot read exist).
- Task 20 completed (config-admin handler pattern to follow).

## Out of Scope

- Do not add P1 pool-policy fields not in the `docs/planning/26` P0 pool schema.
- Do not implement rollback (P1) or tenant-authored fingerprint profiles.

## Expected Files

- Modify: `internal/control/postgres_config_list_store.go` (add `ListExecutorPools`, `GetExecutorPool`).
- Modify: `internal/control/config_admin_handlers.go` (pool CRUD handlers with pagination, stable-ID create, soft
  delete, `expected_config_version` conflict handling).
- Modify: `internal/control/admin_handlers.go` (add the pool store field/interface).
- Modify: `cmd/control/main.go` (register routes; construct the pool-policy provider from the snapshot instead of
  passing `nil`).
- Test: `internal/control/config_admin_handlers_test.go`, routing test covering pool-sourced policy.

## Steps

- [x] Read all required planning docs.
- [x] Add `ListExecutorPools`/`GetExecutorPool` to the config list store.
- [x] Add `GET/POST/PUT/DELETE /api/v1/config/executor-pools` handlers: client-supplied stable IDs (per
      `docs/planning/26`), soft delete, `expected_config_version` -> HTTP 409 `conflict`, and the RBAC roles from the
      `docs/planning/26` table (`tenant_admin` write; `tenant_admin`/`operator`/`viewer` read).
- [x] Increment tenant config version, record the actor audit source, and publish invalidation on every successful
      write (same pattern as routing/deny/injection handlers in task 20).
- [x] Replace `NewStaticPoolPolicyProvider(nil)` in the dispatcher with a provider built from the captured snapshot's
      `ExecutorPools` so degraded-pool policy is configuration-driven.
- [x] Add tests for: pool CRUD with stable IDs and soft delete; tenant isolation; pagination defaults; version
      conflict; a pool created via the API appearing in the assembled snapshot and influencing routing.
- [x] Run the focused tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- `/api/v1/config/executor-pools` CRUD works with client-supplied stable IDs, soft delete, and 409 conflict on
  version mismatch.
- Pools created through the API are visible in the assembled tenant snapshot and consumed by routing.
- Degraded-pool policy is sourced from configuration, not a `nil` provider.

## Handoff Notes

- Document each endpoint and its roles.
- Note how the pool-policy provider is now populated and any degraded-policy default.

## Stop Conditions

- Stop before adding P1 pool fields or rollback.
- Stop if a requested endpoint is not in the `docs/planning/26` P0 table.
- Stop if a deferral would have no owning task file.

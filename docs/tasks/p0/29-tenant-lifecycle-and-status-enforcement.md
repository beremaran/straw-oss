# 29 - Tenant Lifecycle API and Status Enforcement

Status: done

## Objective

Implement the remaining P0 tenant endpoints (list, get, update, soft-delete), persist and load tenant `name` and
`rate_limit_ceiling`, and enforce tenant status during authentication so a suspended or soft-deleted tenant's keys
fail with `tenant_not_found`.

## Context (gap being closed)

The 2026-07-04 review found only `POST /tenants` is implemented; `GET /tenants`, `GET /tenants/{id}`,
`PUT /tenants/{id}`, and `DELETE /tenants/{id}` from the `docs/planning/26` P0 table are missing. Two consequences:
(1) the `Authenticator` never checks tenant status, so `tenant_not_found` (a `docs/planning/14` P0 code for a
missing/deleted tenant) is unreachable and a disabled tenant's keys keep executing; (2) the `tenants` table carries
only `id/status/timestamps`, so `Tenant.RateLimitCeiling` is never persisted or loaded — the rate-limit ceiling
enforcement wired in tasks 13/26 is inert in the real binary. Task 18 explicitly deferred these columns to a later
task; this is that task.

Vocabulary reconciliation: migration `0001_init.sql` gave `tenants` a status CHECK of
`('active', 'disabled', 'deleted')` and a `soft_deleted_at` column, but `docs/planning/26` defines the tenant status
enum as `active | suspended | deleted` and the shared soft-delete contract sets `deleted_at` (which migration 0002
already uses for config resources). The planning doc wins: this task's migration renames the column and replaces the
CHECK, migrating any `'disabled'` rows to `'suspended'`.

## Required Planning Docs

- `docs/planning/06-identity-roles-and-tenant-isolation.md` (tenant resolution, roles)
- `docs/planning/26-config-management-api-surface.md` (tenant endpoint table, RBAC, soft-delete)
- `docs/planning/14-canonical-error-registry.md` (`tenant_not_found`)
- `docs/planning/20-rate-limits-and-quotas.md` (`rate_limit_ceiling` semantics)
- `docs/planning/25-dynamic-configuration.md` (cache invalidation on revocation/disable)
- `docs/planning/21-state-and-storage.md`

## Prerequisites

- Task 07 completed (auth/RBAC).
- Task 18 completed (Postgres identity stores; documented the deferred columns).
- Task 19 and Task 21 completed (snapshot + invalidation).

## Out of Scope

- Do not add P1 tenant metadata-storage-policy fields beyond what P0 rate-limit/ceiling behavior needs.
  *(Correction 2026-07-06: `default_timeout_ms`, `max_timeout_ms`, `metadata_query_storage`, and
  `metadata_path_storage` are P0 per `docs/planning/26` "P0 Config Resource Schemas"; they are owned by
  `docs/tasks/p0/45-tenant-p0-schema-fields.md`.)*
- Do not implement config rollback (P1).
- Do not move worker runtime state or add multi-Control coordination.

## Expected Files

- Create: `migrations/postgres/0003_tenant_fields.sql` (add `name text`, `rate_limit_ceiling` column(s); replace the
  tenants status CHECK with `('active', 'suspended', 'deleted')` migrating `'disabled'` rows to `'suspended'`; rename
  `soft_deleted_at` to `deleted_at` per `docs/planning/26`; idempotent and re-appliable).
- Modify: `internal/control/tenant_store.go` (extend `TenantStore` with List/Update/SoftDelete; load `Name`,
  `RateLimitCeiling`).
- Modify: `internal/control/postgres_tenant_store.go` (persist/load the new columns; status update; soft delete;
  list with pagination).
- Modify: `internal/control/identity.go` (Authenticator resolves the tenant for tenant-scoped keys and rejects when
  status is not `active`, mapping to `tenant_not_found` without leaking which check failed).
- Modify: `internal/control/admin_handlers.go` (add `ListTenants`, `GetTenant`, `UpdateTenant`, `SoftDeleteTenant`).
- Modify: `cmd/control/main.go` (register the routes).
- Test: `internal/control/admin_handlers_test.go`, `internal/control/auth_test.go`,
  `internal/control/postgres_store_test.go`.

## Steps

- [x] Read all required planning docs.
- [x] Add the migration for `name` and `rate_limit_ceiling`, the `('active', 'suspended', 'deleted')` status CHECK
      (migrating `'disabled'` rows), and the `soft_deleted_at` -> `deleted_at` rename; keep it idempotent and
      re-appliable.
- [x] Persist and load `name` and `rate_limit_ceiling` in the Postgres tenant store; add status update and soft delete
      (`status = 'deleted'`, `deleted_at = now()`), and a paginated list.
- [x] Enforce tenant status in authentication: a tenant-scoped key whose tenant is `suspended` or `deleted` fails with
      `tenant_not_found`, collapsing all cases so callers cannot probe tenant state.
- [x] Add `GET /tenants` and `PUT/DELETE /tenants/{id}` (system_admin) and `GET /tenants/{id}` (system_admin plus the
      owning tenant's roles), per the `docs/planning/26` RBAC column. Register the new routes under the canonical
      `/api/v1/config` base path; task 36 moves the existing bare registrations (shared `cmd/control/main.go`, so land
      36 first or coordinate).
- [x] On disable/soft-delete, force config-cache/auth invalidation before returning success (same pattern as API key
      revocation in task 07/21).
- [x] Add tests for: each endpoint's RBAC; tenant isolation on `GET /tenants/{id}`; suspended/deleted tenant key
      returns `tenant_not_found`; `rate_limit_ceiling` persists and a rate-limit write above the ceiling is rejected
      with `invalid_request` end to end (proving the ceiling is now live).
- [x] Run the focused tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- All five P0 tenant endpoints exist with the `docs/planning/26` RBAC.
- A suspended or soft-deleted tenant's API key is rejected with `tenant_not_found`.
- The tenants schema matches `docs/planning/26` vocabulary: status enum `active | suspended | deleted` and
  `deleted_at`.
- `rate_limit_ceiling` is persisted, loaded, and enforced: an over-ceiling rate-limit write is rejected.
- Migrations apply cleanly to a fresh database and are re-appliable.

## Handoff Notes

- Document the new columns and which application code reads them.
- Note the exact status values that gate authentication.

## Stop Conditions

- Stop before adding P1 tenant fields or rollback.
- Stop if a schema change would conflict with task 04's documented model without reconciling it here.
- Stop if a deferral would have no owning task file.

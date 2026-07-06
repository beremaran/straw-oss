# Handoff

Task: `docs/tasks/p0/29-tenant-lifecycle-and-status-enforcement.md`

## Changed

- `migrations/postgres/0003_tenant_fields.sql` (new): adds `tenants.name`,
  `rate_limit_ceiling_window_seconds`/`rate_limit_ceiling_max_requests`, and
  `config_version`; replaces the `status` CHECK with
  `active | suspended | deleted` (migrating `disabled` rows to `suspended`);
  renames `soft_deleted_at` to `deleted_at`. Idempotent and re-applied twice
  via `./scripts/check-postgres-migrations.sh` with no errors.
- `internal/control/tenant_store.go`: `Tenant` gains `Name`, `UpdatedAt`,
  `DeletedAt`, `RateLimitCeiling`, `ConfigVersion`; added
  `TenantStatusSuspended`, `ErrTenantVersionConflict`; `TenantStore` gained
  `List`/`Update`/`SoftDelete`; `InMemoryTenantStore` implements all three
  with the same not-found/conflict semantics as the Postgres store.
- `internal/control/postgres_tenant_store.go`: `Create` now persists `name`
  and `config_version=0`; `Get` loads the new columns; added `List` (paged,
  `created_at DESC, id ASC`), `Update` (optimistic concurrency on the
  tenant's own `config_version`, distinct from the `tenant_config_versions`
  snapshot version), and `SoftDelete`.
- `internal/control/identity.go`: `Authenticator` gained an optional
  `TenantStore` (`SetTenantStore`); `Authenticate` now rejects a
  tenant-scoped key whose tenant is not `active` with `ErrTenantNotFound`,
  collapsing missing/suspended/deleted into one response. A nil tenant store
  (untouched test constructors) skips the check, preserving old behavior.
- `internal/control/handler.go`: the data-plane `POST /api/v1/requests` path
  now maps `ErrTenantNotFound` to the `tenant_not_found` code (previously it
  would have fallen into the generic `auth_failure` default case).
- `internal/control/admin_handlers.go`: added `ListTenants`, `GetTenant`,
  `UpdateTenant`, `SoftDeleteTenant`; `writeAuthOrRBACError` maps
  `ErrTenantNotFound` to 401; `UpdateTenant`/`SoftDeleteTenant` call the
  existing `bumpTenantVersion` + `ConfigCache.PublishInvalidation` before
  returning success, matching the API-key-revocation invalidation pattern.
- `cmd/control/main.go`: constructs one `control.NewPostgresTenantStore` per
  process and wires it into both `Authenticator`s (`buildControlMux`'s
  data-plane authenticator and `AdminHandlers.Authenticator`) via
  `SetTenantStore`, and into `AdminHandlers.Tenants`. Registers
  `GET/PUT/DELETE /api/v1/config/tenants(/{id})` under the canonical base
  path per the task note (task 36 still owns moving the pre-existing bare
  `POST /tenants` and other bare routes). Split `serveAdminRoutes` into
  `serveIdentityRoutes`/`serveConfigResourceRoutes`/`serveWorkerAdminRoutes`
  to stay under the `funlen` limit.

## Verification

```sh
make check
```

Result: `fmt-check` clean, `go test ./...` passes, `golangci-lint run
--max-issues-per-linter 0 --max-same-issues 0` reports 0 issues.

Additionally ran against a live Postgres (`docker compose up -d postgres`,
`STRAW_TEST_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/straw?sslmode=disable`):
- `./scripts/check-postgres-migrations.sh` — migration set (including the
  new 0003) applies cleanly to a fresh database and re-applies without
  error.
- `go test ./internal/control/... -run TestPostgres` — all Postgres-backed
  tests pass, including the new tenant store tests
  (persistence/list/update/version-conflict/soft-delete).

## Reviewer Start Points

- `internal/control/identity.go` (`checkTenantActive`) — the auth
  enforcement change.
- `internal/control/admin_handlers.go` (tenant lifecycle section, top of
  file) — the new endpoints and RBAC.
- `internal/control/postgres_tenant_store.go` — optimistic concurrency and
  soft-delete semantics.
- `migrations/postgres/0003_tenant_fields.sql` — schema/vocabulary
  reconciliation.

## Remaining Work

- The pre-existing bare route registrations (`POST /tenants`, `PUT /tenants/{id}/quotas`,
  `GET/PUT /rate-limits`, etc.) are unchanged; moving them under
  `/api/v1/config` is task 36's scope, not this task's.
- P1 tenant fields (`default_timeout_ms`, `max_timeout_ms`,
  `metadata_query_storage`, `metadata_path_storage`) are out of scope per
  the task's "Out of Scope" section and were not added.
  **[Update 2026-07-06: the "P1" label was wrong — `docs/planning/26` lists all four under "P0 Config Resource
  Schemas". Now owned by `docs/tasks/p0/45-tenant-p0-schema-fields.md`.]**

## Blockers

- None.

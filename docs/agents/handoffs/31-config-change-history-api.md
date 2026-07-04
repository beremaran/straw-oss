# Handoff

Task: `docs/tasks/p0/31-config-change-history-api.md`

## Changed

- `internal/control/audit.go`: added `ListTenantPage(ctx, tenantID, limit, offset)` to the `AuditStore`
  interface and its in-memory implementation, sorted `created_at` descending then `id` ascending, matching
  the shared list contract (`docs/planning/26`). Kept the existing unpaginated `ListTenant` (still used by a
  Postgres integration test) rather than replacing it.
- `internal/control/postgres_audit_store.go`: added the Postgres `ListTenantPage` implementation
  (`ORDER BY created_at DESC, id ASC LIMIT/OFFSET`).
- `internal/control/config_admin_handlers.go`: added `ListChanges` (`GET /api/v1/config/changes`), RBAC
  `tenant_admin`/`operator`/`viewer` via the existing `authorizeConfig` helper, using the shared
  `parsePagination` limit/offset clamp. Response DTO (`configChangeResponse`) only carries actor/resource/action/
  timestamp fields already present on `AuditRecord` — no secret material exists to redact at this layer, since
  redaction happens at write time (task 19).
- `cmd/control/main.go`: registered `GET /api/v1/config/changes` in `serveConfigResourceRoutes`.
- `internal/control/postgres_config_store.go`: extracted `resourceTypeRoutingRule = "routing_rule"` as a
  constant (goconst tripped once the same literal also appeared in the new test's assertion); replaced the
  three existing literal call sites with the constant. No behavior change.
- `internal/control/config_admin_handlers_test.go`: added `TestListChangesRBACAndContent`,
  `TestListChangesTenantIsolation`, `TestListChangesPaginationDefaultsAndBounds`.

## Verification

```sh
go test ./internal/control/... -run 'TestListChanges|TestRoutingRule|TestExecutorPool|TestPostgresAuditStore' -v
go test ./...
make check
```

Result: all pass. `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`: 0 issues.
`TestPostgresAuditStoreActorRecords` (and any other `STRAW_TEST_POSTGRES_DSN`-gated test) skips without a live
Postgres DSN, same as before this change — no new Postgres-only coverage was added for `ListTenantPage` beyond
the handler-level in-memory tests.

## Reviewer Start Points

- `internal/control/config_admin_handlers.go` (`ListChanges`, `configChangeResponse`)
- `internal/control/audit.go` (`AuditStore.ListTenantPage`, `InMemoryAuditStore.ListTenantPage`)
- `internal/control/postgres_audit_store.go` (`postgresAuditStore.ListTenantPage`)

## Pagination behavior and fields returned

- `GET /api/v1/config/changes?limit=&offset=`: `limit` default 50, clamped to max 200 (`clampConfigListLimit`,
  shared with every other config list endpoint); `offset` default 0; invalid/negative `limit` falls back to the
  default via the existing `parsePagination` helper (unchanged shared behavior, not new to this task).
- Sort order: `created_at` DESC, then `id` ASC (shared contract).
- Response is a JSON array of `{id, actor_type, actor_id, resource_type, resource_id, action, created_at}`.
  `created_at` is RFC3339. No tenant ID field is echoed (the caller already knows its own tenant), and no field
  carries secret/credential material — the stored rows are redacted at write time by task 19's write paths.
- Tenant isolation is enforced the same way as every other config list handler: the query is scoped to
  `identity.TenantID` at the store layer, never to a caller-supplied tenant.

## Remaining Work

- The ClickHouse `config_audit_events` mirror is explicitly out of scope here and owned by
  `docs/tasks/p0/32-request-outcome-and-worker-audit-telemetry.md` (per this task's own Out of Scope section).
- Config rollback (P1) and any mutation on this endpoint remain unbuilt by design.

## Blockers

- None.

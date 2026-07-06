# Handoff

Task: `docs/tasks/p0/46-tenant-p0-schema-fields.md`

## Changed

- `migrations/postgres/0010_tenant_p0_fields.sql`: adds `default_timeout_ms`, `max_timeout_ms`, `metadata_query_storage`, and `metadata_path_storage` with planning defaults and CHECK constraints.
- `internal/control/tenant_store.go`, `internal/control/postgres_tenant_store.go`, `internal/config/snapshot.go`, `internal/control/postgres_snapshot_store.go`: tenant P0 fields now normalize, validate, persist, load, and flow into tenant snapshots.
- `internal/control/admin_handlers.go`: `GET/PUT /api/v1/config/tenants/{id}` now returns and accepts all four canonical fields; invalid timeout bounds or storage policies return `invalid_request`.
- `internal/control/handler.go`, `internal/control/proxy_handler.go`, `internal/control/dispatcher.go`, `internal/control/tunnel_dispatcher.go`, `cmd/control/main.go`: request validation uses `min(static max_timeout_ms, tenant max_timeout_ms)`, and missing request timeouts default from the tenant snapshot clamped by the static ceiling.
- `internal/control/request_metadata.go`: `target_url` sanitization now applies tenant query/path storage policy; hash mode uses SHA-256 rendered as `sha256:<hex>`.
- `docs/tasks/p0/29-tenant-lifecycle-and-status-enforcement.md` and `docs/agents/handoffs/29-tenant-lifecycle-and-status-enforcement.md`: stale "P1 fields" wording replaced with a pointer to task 46.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (`019f3689-cbb6-7381-99ca-f0e99a956865`), after the verifier-requested query-hash test was added.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `GET/PUT /api/v1/config/tenants/{id}` round-trips all four fields; invalid enum values and `default_timeout_ms > max_timeout_ms` are rejected with `invalid_request`. | VERIFIED | `internal/control/admin_handlers.go:122`, `internal/control/admin_handlers.go:133`, `internal/control/tenant_store.go:69`, `internal/control/postgres_tenant_store.go:81`, `migrations/postgres/0010_tenant_p0_fields.sql:3` | `TestUpdateTenantPersistsRateLimitCeilingAndForcesInvalidation`, `TestUpdateTenantRejectsInvalidP0Fields`, `TestPostgresTenantStoreUpdatePersistsNameAndCeiling` |
| A request with no timeout gets the tenant's `default_timeout_ms`; a request above the tenant's `max_timeout_ms` is rejected. | VERIFIED | `internal/control/handler.go:94`, `internal/control/dispatcher.go:1010`, `cmd/control/main.go:709` | `TestDispatcherDefaultTimeoutUsesTenantPolicyClampedByStaticMax`, `TestHandlerRejectsTimeoutAboveTenantMax` |
| Stored `target_url` obeys each storage mode; with defaults, query is dropped and path is hashed. | VERIFIED | `internal/control/request_metadata.go:282`, `internal/control/request_metadata.go:292`, `internal/control/request_metadata.go:303`, `internal/control/postgres_snapshot_store.go:199` | `TestSanitizeTargetURLDefaultsDropQueryHashPath`, `TestSanitizeTargetURLStorageModes` |
| The "P1 fields" wording in task 29's file and handoff is replaced with a pointer to this task. | VERIFIED | `docs/tasks/p0/29-tenant-lifecycle-and-status-enforcement.md:44`, `docs/agents/handoffs/29-tenant-lifecycle-and-status-enforcement.md:80` | Diff inspection |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Tenant `default_timeout_ms` field, default `60000`. | implemented | `migrations/postgres/0010_tenant_p0_fields.sql:3`, `internal/control/tenant_store.go:25` |
| Tenant `max_timeout_ms` field, default `300000`. | implemented | `migrations/postgres/0010_tenant_p0_fields.sql:4`, `internal/control/tenant_store.go:26` |
| Tenant `metadata_query_storage` field: `drop | hash | store`, default `drop`. | implemented | `migrations/postgres/0010_tenant_p0_fields.sql:5`, `internal/control/request_metadata_test.go:98` |
| Tenant `metadata_path_storage` field: `store | hash | drop`, default `hash`. | implemented | `migrations/postgres/0010_tenant_p0_fields.sql:6`, `internal/control/request_metadata_test.go:81` |
| Request `timeout_ms` capped by tenant and deployment limits; below `1000` rejected. | implemented | `internal/control/handler.go:94`, `internal/control/request.go:350` |
| Missing request timeout defaults from tenant policy and is clamped by deployment ceiling. | implemented | `internal/control/dispatcher.go:1010`, `internal/control/dispatcher_test.go:570` |
| `target_url` query is dropped unless tenant policy allows storage; query hash is available for correlation. | implemented | `internal/control/request_metadata.go:292`, `internal/control/request_metadata_test.go:112` |
| `target_url` path defaults to stable hash and supports store/drop. | implemented | `internal/control/request_metadata.go:298`, `internal/control/request_metadata_test.go:81`, `internal/control/request_metadata_test.go:98` |

## Verification

```sh
go test ./internal/control -run 'TestSanitizeTargetURLStorageModes|TestSanitizeTargetURLDefaultsDropQueryHashPath'
make check
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
```

Result:

- Postgres-backed tests: ran against dedicated `straw_test` and passed.
- Live compose verification: skipped because only `straw-postgres-1` was running; Control/NATS/Egress were not available without starting/reconfiguring the dev stack. The in-process request dispatch tests and Postgres-backed tests passed.

## Reviewer Start Points

- `internal/control/admin_handlers.go`
- `internal/control/request_metadata.go`
- `internal/control/handler.go`
- `internal/control/dispatcher.go`
- `internal/control/postgres_tenant_store.go`
- `migrations/postgres/0010_tenant_p0_fields.sql`

## Remaining Work

- None.

## Blockers

- None.

# Handoff

Task: `docs/tasks/p1/07-config-rollback-api.md`

## Changed

- Added `POST /api/v1/config/rollback` wiring and handler under the canonical config base path.
- Added Postgres rollback execution that reads versioned `config_audit_source` rows, applies rollback-safe resources in one transaction, writes new audit rows, and publishes invalidation after commit.
- Added `config_audit_source.config_version` migration and writes so rollback has a durable version boundary.
- Added focused handler/store tests for RBAC, invalidation, successful rollback, conflict, redacted-secret rejection, and audit versioning.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Rollback creates a new tenant config version | VERIFIED | `internal/control/config_rollback.go:128`, `internal/control/config_rollback.go:159`, `internal/control/config_rollback.go:476` | `TestPostgresConfigRollbackRestoresAuditBackedResources` |
| Secret values are never restored from redacted audit records | VERIFIED | `internal/control/config_rollback.go:197`, `internal/control/config_rollback.go:419` | `TestPostgresConfigRollbackRejectsRedactedInjectionPolicy` |
| Version conflicts return canonical `conflict` | VERIFIED | `internal/control/config_rollback.go:137`, `internal/control/config_admin_handlers.go:1234` | `TestRollbackConfigVersionConflict` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P1 `POST /api/v1/config/rollback`, role `tenant_admin` | implemented | `cmd/control/main.go:828`, `internal/control/config_admin_handlers.go:1198` |
| Rollback request fields `expected_config_version`, `target_config_version`, `reason` | implemented | `internal/control/config_rollback.go:29`, `internal/control/config_admin_handlers.go:1212` |
| Version mismatch returns HTTP 409 `conflict` with current-version details where available | implemented | `internal/control/config_rollback.go:137`, `internal/control/config_admin_handlers.go:1234`, `internal/control/config_admin_handlers_test.go:647` |
| Rollback creates a new tenant config version and does not reuse the target version | implemented | `internal/control/config_rollback.go:159`, `internal/control/postgres_config_store_test.go:242` |
| Config writes are atomic with tenant version and audit rows in Postgres | implemented | `internal/control/config_rollback.go:131`, `internal/control/config_rollback.go:450`, `internal/control/config_rollback.go:476` |
| `config_audit_source` is durable audit source for config changes | implemented | `migrations/postgres/0008_config_audit_version.sql:1`, `internal/control/postgres_config_store.go:263` |
| Rollback restores only values present in audit source records | implemented | `internal/control/config_rollback.go:197`, `internal/control/config_rollback.go:150` |
| Secret fields cannot be restored from redacted audit records | implemented | `internal/control/config_rollback.go:419`, `internal/control/postgres_config_store_test.go:281` |
| Redis config invalidation after successful write | implemented | `internal/control/config_admin_handlers.go:1226`, `internal/control/config_admin_handlers_test.go:611` |
| Rollback-safe resources documented | implemented | Rollback-safe: routing rules, executor pools, deny rules, quota config, and rate-limit config. Secret-bearing injection policies are rejected when rollback would cross them. |

## Verification

```sh
go test ./internal/control ./cmd/control
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
make check
```

Result:

- Postgres-backed tests: ran against `straw_test`.
- Live compose verification: skipped because this task changes the config API/store path, not the runtime request path.

## Reviewer Start Points

- `internal/control/config_rollback.go`
- `internal/control/config_admin_handlers.go`
- `migrations/postgres/0008_config_audit_version.sql`

## Remaining Work

- None. Rollback is intentionally limited to audit-backed, rollback-safe config resources; secret-bearing injection-policy rollback rejects with `invalid_request` so operators must reapply those values through the normal endpoint.

## Blockers

- None.

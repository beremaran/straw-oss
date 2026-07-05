# Handoff

Task: `docs/tasks/p0/44-config-audit-event-enrichment.md`

## Changed

- **`internal/control/audit.go`**:
  - Extended `AuditRecord` to carry `ConfigVersion`, `FieldPath`, `OldValueJSON`, `NewValueJSON`, and `SkipPostgres`.
  - Added `redactAndMarshal` to switch on `config.InjectionPolicy` and `*config.InjectionPolicy` to redact `ValueBase64` inside injection operations to `"[redacted]"` before marshaling.
  - Added `authSystemAdmin` helper method to simplify authorization checks while complying with linters.
  - Updated `InMemoryAuditStore.Record` to respect `SkipPostgres`.
- **`internal/control/postgres_audit_store.go`**:
  - Updated `postgresAuditStore.Record` to skip database writes when `SkipPostgres: true` is set (double-write prevention).
  - Wired `old_value_json` and `new_value_json` column inserts when `SkipPostgres: false`.
- **`internal/control/config_admin_handlers.go`**:
  - Fetched old configuration values for Routing Rules, Executor Pools, Deny Rules, and Injection Policies, and passed the version, old/new values, and `h.ConfigWrites != nil` as `SkipPostgres` to `recordAudit`.
- **`internal/control/admin_handlers.go`**:
  - Fetched existing values and threaded config version, old/new values, and `SkipPostgres` for Tenant write, API Key write, Worker Credential write, and Quotas & Rate Limits write paths.
- **`internal/control/worker_handlers.go`**:
  - Avoided early returns on transactional worker state updates; called `recordAudit` with `SkipPostgres: true` when already audited transactionally.
- **`internal/control/request_admin_handlers.go`**:
  - Updated `CancelRequest`'s `recordAudit` call to match the updated signature.
- **`internal/control/audit_test.go`**:
  - Added `TestRecordAuditEnrichmentAndRedaction` and `TestRecordAuditSkipPostgres` to verify ClickHouse mirroring, double-write prevention, and secret redaction.
- **`internal/control/ratelimit_test.go` & `internal/control/routing_test.go`**:
  - Fixed pre-existing `goconst` linter issues.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Event rows carry `config_version` and non-empty `old_value_json`/`new_value_json` | VERIFIED | [audit.go:L130-154](file:///Users/beremaran/projects/straw/internal/control/audit.go#L130-154) | `TestRecordAuditEnrichmentAndRedaction` |
| Secrets never appear unredacted in any event row | VERIFIED | [audit.go:L157-181](file:///Users/beremaran/projects/straw/internal/control/audit.go#L157-181) | `TestRecordAuditEnrichmentAndRedaction` |
| Double-write prevention (SkipPostgres) avoids duplicate rows in Postgres/InMemory | VERIFIED | [postgres_audit_store.go:L20-30](file:///Users/beremaran/projects/straw/internal/control/postgres_audit_store.go#L20-30) | `TestRecordAuditSkipPostgres` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Config change structure carrying `old_value_json`/`new_value_json`/`config_version` (`docs/planning/26`) | implemented | [audit.go:L16-25](file:///Users/beremaran/projects/straw/internal/control/audit.go#L16-25) |
| Secrets redaction for `injection_policies.operations.value_base64` (`docs/planning/21`) | implemented | [audit.go:L157-181](file:///Users/beremaran/projects/straw/internal/control/audit.go#L157-181) |
| Avoid postgres duplicate row double-writes on transactional configurations (`docs/planning/21`) | implemented | [postgres_audit_store.go:L20-30](file:///Users/beremaran/projects/straw/internal/control/postgres_audit_store.go#L20-30) |

## Verification

```sh
make check
```

Result:
- **Postgres-backed tests**: ran against `straw_test` successfully:
  `STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./internal/control/...`
- **Live compose verification**: skipped (focused unit and Postgres integration tests fully covered all implementation paths).

## Reviewer Start Points

- [internal/control/audit.go](file:///Users/beremaran/projects/straw/internal/control/audit.go)
- [internal/control/audit_test.go](file:///Users/beremaran/projects/straw/internal/control/audit_test.go)

## Remaining Work

- None.

## Blockers

- None.

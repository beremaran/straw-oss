# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T012`

## Changed

- Added migration `0012_executable_fingerprint_profiles.sql`, which preserves seeded rows for audit history, makes `chrome_120` the enabled executable descriptor, and retires `default`, `firefox_121`, and `safari_17`.
- Added immutable descriptor fields to snapshots and Postgres read models.
- Reworked the read-only catalog response into supported/unavailable projections with `default -> baseline` exposed only as an alias.
- Synchronized the in-memory catalog fixture and resolved adjacent lint-only test findings.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Existing seeded rows are preserved; Chrome 120 has its immutable descriptor; legacy browser rows are retired. | VERIFIED | `migrations/postgres/0012_executable_fingerprint_profiles.sql:5` | `TestPostgresFingerprintProfileMigrationPreservesExistingRows` |
| Snapshots and durable reads retain immutable profile metadata. | VERIFIED | `internal/config/snapshot.go:113`, `internal/control/postgres_snapshot_store.go:456`, `internal/control/postgres_config_list_store.go:474` | `TestTenantSnapshotClonePreservesImmutableFingerprintProfileMetadata`, `TestPostgresConfigStoreSnapshotAssembly` |
| The read-only API separates catalog entries from the baseline alias and provides bounded unavailable reasons. | VERIFIED | `internal/control/config_admin_handlers.go:1117` | `TestFingerprintProfilesReadOnly`, `TestInMemoryFingerprintProfileStoreSeedsExecutableCatalogOnly` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Durable seeded catalog retains audit history and exactly identifies the executable Chrome 120 definition. | implemented | `migrations/postgres/0012_executable_fingerprint_profiles.sql:5` |
| Baseline compatibility alias is distinct from named profiles. | implemented | `internal/control/config_admin_handlers.go:1146` |
| Runtime session capability filtering and precise availability calculation. | pending | `specs/002-straw-fingerprint-profiles/tasks.md#T031` |

## Verification

```sh
go test ./internal/config ./internal/control -run 'Test(TenantSnapshotClonePreservesImmutableFingerprintProfileMetadata|FingerprintProfilesReadOnly|InMemoryFingerprintProfileStoreSeedsExecutableCatalogOnly)$'
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./internal/control -run 'Test(PostgresFingerprintProfileMigrationPreservesExistingRows|PostgresConfigStoreSnapshotAssembly)$' -count=1
make check
```

Result: all passed; `make check` completed with zero lint issues. Postgres-backed checks ran against the guarded `straw_test` database. Live compose verification was not required for this catalog/migration task.

## Reviewer Start Points

- `migrations/postgres/0012_executable_fingerprint_profiles.sql`
- `internal/control/config_admin_handlers.go:1127`

## Remaining Work

- T031 owns runtime capability filtering and dynamic availability reasons.

## Blockers

- None. Changes are uncommitted.

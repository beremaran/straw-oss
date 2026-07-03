# 18 - Postgres Foundation and Identity Stores

Status: done

## Objective

Add the real Postgres connection foundation and Postgres-backed identity stores, then wire Control to use them instead
of in-memory identity state. This task authorizes adding the `github.com/jackc/pgx/v5` dependency.

## Required Planning Docs

- `docs/planning/21-state-and-storage.md`
- `docs/planning/24-static-configuration.md`
- `docs/planning/06-identity-roles-and-tenant-isolation.md`
- `docs/planning/27-security-controls.md`

## Prerequisites

- Task 04 completed.
- Task 07 completed.

## Out of Scope

- Do not implement config-resource stores or tenant snapshot assembly (task 19).
- Do not implement config/admin HTTP APIs beyond identity endpoints already owned by task 07 and backed here.
- Do not implement ClickHouse metadata writes (task 14).
- Do not remove in-memory stores used by unit tests.

## Expected Files

- Create or modify: Postgres connection helper package under existing internal boundaries.
- Create or modify: `internal/control` Postgres-backed API key, tenant, worker credential, and audit stores.
- Modify: `cmd/control/main.go`
- Modify: `go.mod`/`go.sum`
- Test: focused Postgres store tests.

## Steps

- [x] Read all required planning docs.
- [x] Add `github.com/jackc/pgx/v5` and a small connection-pool helper (`internal/postgresx`) using
      `control.database.postgres.*` configuration and `STRAW_POSTGRES_DSN`.
- [x] Migration path: `cmd/control` embeds `migrations/postgres/*.sql` (`migrations.Postgres`) and applies them at
      startup via `postgresx.ApplyMigrations`. `CREATE ... IF NOT EXISTS` makes re-apply idempotent. Verified by
      booting the binary against an empty database (tables auto-created).
- [x] Implement Postgres-backed `TenantStore`. NOTE: the `TenantStore` interface (Create/Get) and the `tenants`
      table only carry id/status/timestamps; `Name`, `rate_limit_ceiling`, soft-deletion, and metadata-storage-policy
      fields have no columns/methods yet and are populated by later tasks (see Handoff Notes / the `Tenant` type doc).
- [x] Implement Postgres-backed `APIKeyStore`: visible-prefix lookup, constant-time verification (via existing
      `VerifyAPIKeySecret`), hashed-secret storage, revocation timestamps, platform-vs-tenant scope, first-platform-key
      bootstrap.
- [x] Implement Postgres-backed `WorkerCredentialStore`, enforcing single-tenant P0 credential scope and status.
- [x] Implement Postgres-backed `AuditStore` for P0 config/auth actor records (redaction is enforced upstream in
      `recordAudit`; `AuditRecord` never carries secret material).
- [x] Wire `cmd/control/main.go` to construct the Postgres stores at runtime. Postgres is now REQUIRED at startup
      (no in-memory fallback), matching "Postgres is the source of truth" in planning doc 21.
- [x] Add tests for persistence, prefix collision, platform key cannot execute requests, tenant isolation (uuid
      round-trip), revocation, worker credential scope, and audit actor records.
- [x] Run focused Postgres identity-store tests (against Postgres 16; 10/10 pass, idempotent).
- [x] Run `make check` (0 lint issues, all tests pass).
- [x] Write a handoff note.

## Tests

- Focused Postgres identity-store tests.
- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- Control runtime uses Postgres-backed tenant, API key, worker credential, and audit stores.
- API key secrets are never stored or logged in plaintext.
- Platform-scoped keys and tenant-scoped keys enforce the Section 6 role boundaries.
- In-memory identity stores remain only for tests and explicitly local fakes.

## Handoff Notes

- Document the migration application path and any bootstrap seed behavior.
- Note that config-resource Postgres stores and snapshot assembly are deferred to `docs/tasks/p0/19-postgres-config-stores-and-snapshot.md`.

## Stop Conditions

- Stop if a schema mismatch requires changing `migrations/postgres/0001_init.sql` outside task 04's documented model.
- Stop before adding user/password authentication.
- Stop if a deferral would have no owning task file.

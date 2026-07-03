# 18 - Postgres Foundation and Identity Stores

Status: not started

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

- [ ] Read all required planning docs.
- [ ] Add `github.com/jackc/pgx/v5` and a small connection-pool helper using `control.database.postgres.*`
      configuration and `STRAW_POSTGRES_DSN`.
- [ ] Decide and implement the P0 migration path: either apply `migrations/postgres/0001_init.sql` at startup or add a
      documented local apply command that startup verifies before serving.
- [ ] Implement Postgres-backed `TenantStore`, honoring tenant status, soft deletion, metadata storage policy fields,
      and `rate_limit_ceiling`.
- [ ] Implement Postgres-backed `APIKeyStore`, including visible-prefix lookup, constant-time secret verification,
      secure hash storage, revocation timestamps, platform-vs-tenant scope rules, and first-platform-key bootstrap
      behavior.
- [ ] Implement Postgres-backed `WorkerCredentialStore`, enforcing single-tenant P0 credentials and credential status.
- [ ] Implement Postgres-backed `AuditStore` for P0 config/auth actor records with secret-field redaction.
- [ ] Wire `cmd/control/main.go` to construct these Postgres stores in place of the identity in-memory stores at
      runtime.
- [ ] Add tests for store persistence, prefix collision handling, platform key cannot execute requests, tenant
      isolation, revocation, worker credential scope, and audit redaction.
- [ ] Run focused Postgres identity-store tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

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

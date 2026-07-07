# Handoff

Task: `docs/tasks/p0/18-postgres-foundation-and-identity-stores.md`

This task was picked up from an incomplete prior session (a local model that aborted mid-run). The Postgres
scaffolding it left **compiled and `go build` passed, but had never been run against a real database** and was
broken in several ways that only a live Postgres exposes. This handoff records both the finish and the corrections.

## Changed

New:

- `internal/postgresx/` (`postgresx.go`, `doc.go`) — pgx connection-pool helper: `Config`, `ResolveDSN`
  (reads `STRAW_POSTGRES_DSN`, defaults the env-var name), `Connect` (pool + ping), `ApplyMigrations` (runs
  `postgres/*.sql` from an `fs.FS`).
- `migrations/embed.go` — `//go:embed postgres/*.sql` → `migrations.Postgres`, so the binary carries its schema
  regardless of working directory.
- `internal/control/postgres_{tenant,apikey,worker_credential,audit}_store.go` — Postgres-backed identity stores.
- `internal/control/postgres_store_test.go` — DSN-gated integration tests (skip without `STRAW_TEST_POSTGRES_DSN`).

Modified:

- `cmd/control/main.go` — construct the Postgres stores at runtime; apply embedded migrations on startup; bootstrap
  the first platform key against Postgres. **Postgres is now required** (see below).
- `internal/config/config.go` — `control.database.postgres.*` config block (`DatabaseConfig`/`PostgresConfig`).
- `internal/control/bootstrap.go` + `internal/control/admin_handlers.go` — resource-ID generator now emits UUIDs
  (see correction #1).
- `.gitignore` — ignore the `control`/`egress` service binaries (a stray `control` binary was left in the tree).
- `go.mod`/`go.sum` — add `github.com/jackc/pgx/v5` (authorized by this task).

## Corrections to the prior session's work

1. **Resource IDs were incompatible with the uuid schema (startup blocker).** `newRandomID("key")` produced
   `"key_<hex>"`, but `api_keys.id`, `tenants.id`, and `worker_credentials.credential_id` are `uuid` columns. Booting
   Control failed at bootstrap: `invalid input syntax for type uuid: "key_..."`. Replaced with `newResourceID()`
   emitting an RFC-4122 v4 UUID (`bootstrap.go`), updating the 5 call sites (bootstrap + 4 identity-create handlers).
   Nothing depended on the old prefix format (no test asserts it, no code parses `kind` back out).
2. **Tenant isolation silently broken on read.** `tenant_id` (uuid) was scanned into `any` then type-asserted to
   `string`; pgx decodes uuid-into-`any` as `[16]byte`, so the assertion always failed and `TenantID` came back
   empty — including the `FindByPrefix` auth hot path. Fixed to scan into `*string` in the API-key and audit stores.
3. **`revoked_at` lost on read.** Scanned into a discarded `any`, then `RevokedAt` set to zero time. Fixed to scan
   into a nullable `*time.Time`.
4. **Silent in-memory fallback contradicted the acceptance criteria.** `main.go` fell back to in-memory identity
   stores when no DSN was set. Planning doc 21 says "Postgres is the source of truth"; criterion says "Control
   runtime uses Postgres-backed stores." Removed the fallback — a missing/failed DSN is now a hard startup error
   (mirroring how NATS is already required).
5. **Rule violations / cleanup.** Removed two `//nolint:nilerr` comments (forbidden); removed a stray compiled
   `control` binary; fixed the DSN-env-name default not being applied before `ResolveDSN`.
6. **Integration tests were broken-by-construction.** They used non-UUID tenant IDs (`"ten_test"`) against uuid
   columns with FKs, and an auth test whose token didn't match the real `sk_live_`/prefix scheme — they would have
   failed the moment they ran. Rewritten with valid UUIDs, FK-parent seeding, truncation for idempotency, and
   `GenerateAPIKey()` for realistic auth.

## Verification

```sh
make check
```

Result: **PASS** — `gofmt` clean, `go test ./...` all pass, `golangci-lint` reports 0 issues.

Beyond `make check` (which skips the DSN-gated tests), verified against a real **Postgres 16** container:

- `STRAW_TEST_POSTGRES_DSN=... go test ./internal/control -run TestPostgres` → 10/10 pass, idempotent across reruns.
- Embedded-migration apply against a fresh database (idempotent second apply) → pass.
- **Full binary boot** (NATS + Postgres, empty DB): embedded migrations created all 14 tables, the platform
  system_admin key bootstrapped into Postgres with a uuid id, `POST /tenants` (authenticated via the Postgres
  `FindByPrefix` path) returned `201` and persisted the tenant with a uuid id, and an audit row
  (`api_key|tenant|create`) was written. Containers/artifacts were torn down afterward.

## Reviewer Start Points

- `cmd/control/main.go` — `runControl` / `openPostgres` / `buildAdminHandlers` (the wiring; Postgres now required).
- `internal/postgresx/postgresx.go` — `ResolveDSN` / `Connect` / `ApplyMigrations`.
- `internal/control/postgres_apikey_store.go` — uuid `tenant_id` and `revoked_at` scan handling.
- `internal/control/bootstrap.go` — `newResourceID` (uuid) and bootstrap flow.

## Remaining Work

Out of scope for task 18 (identity stores only); each has an owning task:

- Postgres-backed **quota config**, **rate-limit config**, and **snapshot** stores — still in-memory in
  `buildAdminHandlers` (`NewInMemoryQuotaStore`, `NewInMemoryRateLimitConfigStore`, `NewInMemorySnapshotStore` behind
  `ConfigCache`). Owned by `docs/tasks/p0/19-postgres-config-stores-and-snapshot.md` (which lists task 18 as a
  prerequisite).
- Tenant fields not yet persisted: `Name`, `RateLimitCeiling`, soft-deletion, metadata-storage-policy — no columns
  in the `tenants` table and no methods on `TenantStore` yet. The `Tenant` type documents these as later-task work;
  the config-resource side is task 19.
  [Update 2026-07-07 sweep: resolved by `docs/tasks/p0/29-tenant-lifecycle-and-status-enforcement.md` and
  `docs/tasks/p0/46-tenant-p0-schema-fields.md`; `TenantStore` now persists name/status/soft-deletion,
  timeout defaults, metadata storage policy, and `rate_limit_ceiling`.]
- The DSN-gated integration tests are not exercised by `make check` (no DSN in CI). Wiring compose + running them in
  CI is `docs/tasks/p0/25-p0-test-matrix-and-compose.md`.
  [Update 2026-07-07 sweep: owned/covered by `docs/tasks/p0/25-p0-test-matrix-and-compose.md`; the DSN-gated tests
  still intentionally skip unless `STRAW_TEST_POSTGRES_DSN` is set.]

Minor, no owning-task action needed (documented here):

- `AuditStore.ListTenant("")` cannot list platform (NULL-`tenant_id`) records — `WHERE tenant_id = $1` rejects an
  empty string against a uuid column. There is no platform-audit-list endpoint in P0; the test verifies the platform
  record via a direct `tenant_id IS NULL` query.

## Blockers

- None.

## Note on scope

Corrections #1 (uuid IDs) and #4 (required Postgres) touch code originally authored under tasks 07/16-17. They are
not new features — they are the minimal reconciliation required for task 18's own acceptance criterion ("Control
runtime uses Postgres-backed stores"): without #1 the process cannot start against Postgres, and #4 is what makes
"uses Postgres" true rather than optional. Flagging here for reviewer visibility.

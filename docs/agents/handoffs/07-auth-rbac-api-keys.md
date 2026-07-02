# Handoff

Task: `docs/tasks/p0/07-auth-rbac-api-keys.md`

## Changed

- Added `internal/control/apikey.go`: API key generation (32 random bytes, base64url-encoded, `sk_live_` literal
  prefix — 256 bits of entropy), `HashAPIKeySecret` (HMAC-SHA256 with an optional server-side pepper),
  `VerifyAPIKeySecret` (constant-time comparison via `crypto/subtle`), `ExtractKeyPrefix` (first 12 chars used as the
  visible lookup prefix), and `BearerToken` parsing.
- Added `internal/control/identity.go`: `ScopeType`/`Role` types, the `Identity` struct (API key ID, scope, tenant ID,
  role — always derived from the validated key, never from client input), and `Authenticator.Authenticate`, which does
  prefix lookup then checks every same-prefix candidate with constant-time hash comparison, collapsing all failure
  modes (missing header, unknown prefix, wrong secret, revoked key) into a single `ErrAuthFailure` so the failure
  reason can't be probed.
- Added `internal/control/rbac.go`: `RequireRole`, `RequirePlatformScope`, `RequireTenantScope`, `RequireOwnTenant`,
  and `CanExecuteDataPlane` (platform keys always denied; `requester`/`tenant_admin` allowed; `viewer` denied;
  `operator` denied in P0 — see Deviations below).
- Added `internal/control/apikey_store.go`, `worker_credential_store.go`, `tenant_store.go`, `quota_store.go`,
  `snapshot_store.go`: store interfaces plus process-local (`InMemory*`) implementations, mirroring the columns in
  `migrations/postgres/0001_init.sql`. No Postgres/Redis driver dependency exists in this repo yet (task 04/05 only
  shipped schema + interfaces), so this task follows the same pattern established by `config_cache.go` rather than
  introducing a new dependency. A Postgres-backed implementation is future work.
- Added `internal/control/audit.go`: `AuditStore` interface + in-memory implementation, and `recordAudit`, called from
  every mutating admin handler with `actor_type=api_key`, `actor_id=<key ID>`.
- Added `internal/control/bootstrap.go`: `BootstrapFromEnv` seeds the first platform `system_admin` key from an
  operator-supplied env var value (hashed, never generated-and-printed). No-op if a `system_admin` key already exists
  or the env var is unset. See Bootstrap Behavior below.
- Added `internal/control/admin_handlers.go`: `AdminHandlers` implementing the P0 config-management endpoints —
  `POST /tenants` (minimal, `system_admin`-only, enough to prove tenant-key creation is blocked), platform API key
  create/list/revoke, tenant API key create/list/revoke, worker-credential create/list/revoke (forces single-tenant
  scope), `GET /quotas`, `PUT /tenants/{id}/quotas`. Every tenant-scoped config write (tenant key create/revoke,
  worker-credential create/revoke, quota write) bumps the tenant's aggregate `tenant_config_version` through
  `ConfigCache.Save`, which updates the in-process cache and publishes invalidation before the handler returns
  success.
- Modified `internal/control/errors.go`: filled in the named `ErrorCode` constants
  (`TenantNotFound`, `InsufficientPermissions`, `RateLimitExceeded`, `QuotaExhausted`, `InvalidRequest`,
  `DestinationDenied`, `Conflict`, `UnsupportedIngressMode`, `ControlInternalError`) that were previously only present
  as raw map keys.
- Modified `internal/control/handler.go`: `NewRequestHandler` now takes a required `*Authenticator`. `ServeHTTP`
  authenticates and authorizes (`CanExecuteDataPlane`) before validation; a nil authenticator fails closed
  (`ErrAuthFailure`), not open.
- Modified `internal/control/handler_test.go`: added a `newTestHandler` helper that seeds one active tenant
  `requester` key and returns its plaintext token; every existing test now sends `Authorization: Bearer <token>`.
  Behavior asserted by the original 29 tests is unchanged.
- Modified `cmd/control/main.go`: wires `InMemoryAPIKeyStore`, `BootstrapFromEnv` (reading
  `STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY`), `Authenticator`, `InMemorySnapshotStore` + `ConfigCache`, and `AdminHandlers`
  into the HTTP mux (`/tenants`, `/platform-api-keys`, `/api-keys`, `/worker-credentials`, `/quotas`,
  `/tenants/{id}/quotas`) alongside the existing `POST /api/v1/requests` route.
- Added tests: `internal/control/auth_test.go`, `internal/control/admin_handlers_test.go`,
  `internal/control/bootstrap_test.go` (see Tests below).

## Bootstrap Behavior

The first platform `system_admin` API key is seeded from the environment variable
`STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY` (`control.BootstrapSystemAdminEnvVar`). On Control startup,
`control.BootstrapFromEnv`:

1. No-ops if the env var is empty (operators must seed a key via a migration fixture or manual insert instead — this
   intentionally avoids generating and having to print/log a key).
2. No-ops if an active platform `system_admin` key already exists (`APIKeyStore.CountPlatformSystemAdmins`), so it is
   safe to leave the env var set across restarts/redeploys.
3. Otherwise hashes the env var value (HMAC-SHA256 with the pepper) and inserts it as an active platform
   `system_admin` key. The plaintext value is never stored or logged — the operator already knows it because they set
   it.

After bootstrap, all platform API keys (including further `system_admin` keys) are created, listed, and revoked only
through `/platform-api-keys` by an existing `system_admin` key.

The server-side pepper is loaded from `STRAW_API_KEY_PEPPER` (empty pepper is accepted for local development; set a
real value in any shared/production environment).

## Roles and Allowed P0 Actions

| Role | Scope | Data-plane execution (`POST /api/v1/requests`) | Can create tenants | Can manage platform API keys | Can manage tenant API keys / worker credentials | Quota writes | Quota reads |
|------|-------|---|---|---|---|---|---|
| `system_admin` | platform | No (platform keys never execute data-plane requests) | Yes | Yes | No (not tenant-scoped) | Yes (`PUT /tenants/{id}/quotas`) | N/A (no tenant identity) |
| `requester` | tenant | Yes | No | No | No | No | No |
| `viewer` | tenant | No | No | No | No | No | Yes |
| `operator` | tenant | No in P0 (see Deviations) | No | No | No | No | Yes |
| `tenant_admin` | tenant | Yes | No | No | Yes (own tenant only) | No | Yes |

Tenant isolation: every tenant-scoped list/revoke enforces `identity.TenantID == resource.TenantID`; a mismatch
returns `insufficient_permissions` without revealing whether the resource exists (mirrors the
`POST /requests/{request_id}/cancel` precedent in `docs/planning/26`).

## Deviations from the Planning Docs

- `docs/planning/06` marks `operator` data-plane execution as "Optional by tenant policy." No per-tenant policy
  resource exists in this task's scope (or anywhere in the repo yet), so P0 conservatively defaults `operator` to
  **no** data-plane execution (`internal/control/rbac.go`, `CanExecuteDataPlane`). This is a deny-by-default choice,
  not a permission expansion; enabling it per tenant is future work once a tenant policy config resource exists.
- Full `Tenant`/`TenantSnapshot`/`Fingerprint Profile`/`Routing Rule`/`Executor Pool`/`Deny Rule`/`Injection Policy`
  resource schemas from `docs/planning/26` are **not** implemented here — they belong to other P0 tasks (09, and
  tasks not yet on the board for the remaining config surface). `POST /tenants` is implemented only to the minimal
  degree needed to prove "tenant key cannot create tenants" / "system_admin can create tenants" (id, name, status,
  created_at).
- No Postgres or Redis driver dependency was added. Tasks 04/05 only shipped a Postgres migration and interface-only
  config cache (no driver wiring exists anywhere in the repo yet); this task follows the same pattern with
  `InMemory*Store` implementations behind the same interfaces a future Postgres-backed implementation will satisfy.

## Verification

```sh
go test ./internal/control/... -v
make check
```

Result:

- 64 tests pass in `internal/control` (0 failures), including the 29 pre-existing handler/validation tests (now
  exercised with authentication) plus new auth/RBAC/lifecycle/bootstrap tests.
- `make check` (gofmt verification + `go test ./...`) passes across all packages.

Key new tests (in addition to auth/RBAC unit tests for generation, hashing, prefix-collision handling, and
`Authenticate`):

- `TestPlatformAPIKeyLifecycle`, `TestPlatformAPIKeyLifecycleRequiresSystemAdmin`
- `TestPlatformKeyCannotExecuteDataPlaneRequest`, `TestViewerCannotExecuteDataPlaneRequest`,
  `TestUnauthenticatedRequestRejected`
- `TestTenantKeyCannotCreateTenants`, `TestSystemAdminCanCreateTenants`
- `TestAuthenticateRejectsRevokedKey`, `TestRevokeTenantAPIKeyInvalidatesConfigCache`
- `TestActorAuditSourceRecorded`
- `TestTenantIsolationBlocksCrossTenantKeyRevoke`
- `TestQuotaWriteRequiresPlatformKey`
- `TestWorkerCredentialCreateRejectsForeignTenantScope`, `TestWorkerCredentialCreateForcesCallerTenantScope`,
  `TestWorkerCredentialRevokeInvalidatesAcrossTenants`
- `TestBootstrapFromEnvCreatesFirstSystemAdmin`, `TestBootstrapFromEnvNoopWhenAdminExists`,
  `TestBootstrapFromEnvNoopWhenUnset`

## Reviewer Start Points

- `internal/control/apikey.go` — key generation/hashing/verification
- `internal/control/identity.go` — `Authenticator`, `Identity`
- `internal/control/rbac.go` — RBAC helpers and the `operator` deviation note
- `internal/control/admin_handlers.go` — all lifecycle endpoints and cache-invalidation wiring
- `internal/control/bootstrap.go` — bootstrap flow
- `internal/control/handler.go` — data-plane auth integration
- `cmd/control/main.go` — runtime wiring
- `internal/control/auth_test.go`, `internal/control/admin_handlers_test.go`, `internal/control/bootstrap_test.go`

## Remaining Work

- Postgres-backed implementations of `APIKeyStore`, `WorkerCredentialStore`, `TenantStore`, `QuotaStore`,
  `AuditStore`, `SnapshotStore` (currently `InMemory*`), once a Postgres driver dependency is introduced.
- Redis-backed `InvalidationPublisher` wiring for `ConfigCache` (currently `nil` in `cmd/control/main.go`; publish
  calls are no-ops until then).
- Full tenant resource schema (`docs/planning/26` §"Tenant") and remaining config endpoints (routing rules, executor
  pools, deny rules, injection policies, fingerprint profiles, rate limits, config audit list) — out of this task's
  scope, deferred to later tasks.
- Per-tenant `operator` data-plane execution policy, if/when a tenant policy config resource is introduced (see
  Deviations).

## Blockers

- None.

# Handoff

Task: `docs/tasks/p1/08-multi-tenant-worker-credentials.md`

## Changed

- `internal/control/admin_handlers.go`: `POST /api/v1/config/worker-credentials` now accepts platform-scoped `system_admin` callers for multi-tenant credentials, derives `tenant_scope` from scoped `allowed_pools`, and keeps tenant-admin credentials forced to the caller tenant.
- `internal/control/admin_handlers_test.go`: added platform multi-tenant creation and platform scoped-pool validation coverage.
- `internal/control/worker_registry_test.go`: added multi-tenant registration acceptance plus out-of-scope tenant/pool rejection coverage.
- `docs/tasks/p1.md`, `docs/tasks/p1/08-multi-tenant-worker-credentials.md`: marked the verified task done.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Multi-tenant credentials are platform-scoped only. | VERIFIED | `internal/control/admin_handlers.go:729`, `internal/control/admin_handlers.go:736`, `internal/control/admin_handlers.go:1415` | `TestWorkerCredentialCreateSystemAdminMultiTenantScope`, `TestWorkerCredentialCreateSystemAdminRequiresScopedPools` |
| Tenant-scoped credentials preserve P0 behavior. | VERIFIED | `internal/control/admin_handlers.go:738`, `internal/control/admin_handlers.go:1425`, `internal/control/admin_handlers.go:1438` | `TestWorkerCredentialCreateRejectsForeignTenantScope`, `TestWorkerCredentialCreateForcesCallerTenantScope` |
| Routing and registration enforce every credential scope. | VERIFIED | `internal/control/worker_registry.go:599`, `internal/control/worker_registry.go:1075`, `internal/control/worker_registry.go:531`, `internal/control/worker_registry.go:535`, `internal/control/routing.go:285` | `TestRegisterMultiTenantCredentialScope`, `TestRoutingTenantIsolation` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `POST /worker-credentials` P0 tenant-admin behavior remains caller-tenant scoped. | implemented | `internal/control/admin_handlers.go:729`, `internal/control/admin_handlers.go:738`, `internal/control/admin_handlers.go:1425`, `internal/control/admin_handlers_test.go:902` |
| P1 multi-tenant worker credentials are platform-scoped `system_admin` operations. | implemented | `internal/control/admin_handlers.go:729`, `internal/control/admin_handlers.go:736`, `internal/control/admin_handlers.go:1415`, `internal/control/admin_handlers_test.go:927` |
| Multi-tenant `allowed_pools` use scoped `{tenant_id, pool_id}` objects. | implemented | `internal/control/admin_handlers.go:1428`, `internal/control/admin_handlers.go:1450`, `internal/control/admin_handlers_test.go:927` |
| Worker credential schema fields: `id`, `tenant_scope`, `executor_type`, `allowed_pools`, `allowed_capabilities`, `public_key_ed25519_base64`, `status`, `config_version`. | already existed / implemented | Existing response/store shape in `internal/control/admin_handlers.go:688`; create path persists `tenant_scope` and scoped pools at `internal/control/admin_handlers.go:1356` |
| Worker credential binds credential ID, tenant or multi-tenant scope, allowed pool IDs, executor type, signing public key, and status. | implemented | `internal/control/admin_handlers.go:1356`; existing store shape in `internal/control/worker_credential_store.go` |
| Worker registration validates credential status, tenant/pool scope, capability scope, protocol compatibility, safe worker ID, and signature. | already existed / tested | `internal/control/worker_registry.go:572`, `internal/control/worker_registry.go:590`, `internal/control/worker_registry.go:608`, `internal/control/worker_registry.go:621`; new multi-tenant test at `internal/control/worker_registry_test.go:264` |
| A worker cannot register pools, countries, regions, tags, ingress modes, IP types, or other capabilities outside its credential scope. | already existed / tested | `internal/control/worker_registry.go:599`, `internal/control/worker_registry.go:608`; tests at `internal/control/worker_registry_test.go:230`, `internal/control/worker_registry_test.go:247`, `internal/control/worker_registry_test.go:264` |
| A request for tenant A must never route to a worker credentialed only for tenant B. | already existed / verified | `internal/control/worker_registry.go:531`, `internal/control/worker_registry.go:535`; `internal/control/routing_test.go:92` |
| A multi-tenant worker may execute for multiple tenants only when the credential explicitly grants the tenant/pool scope. | implemented / already existed | Creation derives scope from `allowed_pools` at `internal/control/admin_handlers.go:1415`; registration rejects undeclared scope at `internal/control/worker_registry_test.go:287`; routing uses scoped candidates at `internal/control/worker_registry.go:531` |
| Worker credential update endpoint. | out of scope | The required planning docs define create/list/revoke only for worker credentials; no update route exists in `docs/planning/26-config-management-api-surface.md`. No new public API was added. |

## Verification

```sh
go test ./internal/control -run 'TestWorkerCredentialCreate|TestRegisterMultiTenantCredentialScope|TestRoutingTenantIsolation'
go test ./internal/control
make check
```

Result:

- Focused tests: passed.
- `go test ./internal/control`: passed.
- `make check`: passed (`go test ./...` plus `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`, 0 issues).
- Postgres-backed tests: not exercised; the diff does not touch `postgres_*` files or migrations.
- Live compose verification: skipped; the diff does not touch the runtime request path. Registration and routing scope behavior is covered by focused unit tests.

## Reviewer Start Points

- `internal/control/admin_handlers.go`
- `internal/control/admin_handlers_test.go`
- `internal/control/worker_registry_test.go`

## Remaining Work

- None.

## Blockers

- None.

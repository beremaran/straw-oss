# Handoff

Task: `docs/tasks/p1/04-routing-ingress-type-and-worker-capability.md`

## Changed

- Threaded request ingress type from dispatch into routing so REST and proxy-originated requests evaluate the correct mode.
- Added documented ingress constants, route/admin validation, worker credential capability JSON, Postgres persistence, and the `allowed_capabilities_jsonb` migration.
- Added focused dispatcher, routing, registration, admin, and Postgres store tests for ingress-mode matching and capability gates.
- Updated `docs/agents/handoffs/02-http-proxy-ingress.md` to mark its routing/capability deferral closed by this task.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `ingress_type` participates in route matching. | VERIFIED | `internal/control/dispatcher.go:393`, `internal/control/routing.go:236`, `internal/control/request.go:20` | `TestDispatcherRoutesUsingRequestIngressType`, `TestRoutingIngressTypeMatchAndCapability`, `TestRoutingMatchesEveryDocumentedIngressType` |
| Worker credential capability gates prevent incompatible ingress routing. | VERIFIED | `internal/control/postgres_worker_credential_store.go:39`, `internal/control/postgres_worker_credential_store.go:77`, `migrations/postgres/0007_worker_credential_capabilities.sql:2`, `internal/control/worker_registry.go:608`, `internal/control/routing.go:311` | `TestRegisterIngressCapabilityOutOfScope`, `TestRoutingIngressCapabilityDefaultAndUnsupported`, `TestPostgresWorkerCredentialStoreSingleTenant` |
| Existing REST routing stays compatible. | VERIFIED | `internal/control/request.go:165`, `internal/control/proxy_handler.go:158`, `internal/control/routing_test.go:157` | `TestRoutingIngressTypeMatchAndCapability`, `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/10` route `match_conditions.ingress_type` values `rest`, `http_proxy`, `connect`, `mitm`. | implemented | `internal/control/request.go:20`, `internal/control/config_admin_handlers.go:393`, `internal/control/routing.go:236`, `internal/control/routing_test.go:162` |
| `docs/planning/10` request ingress hint participates in rule matching. | implemented | `internal/control/dispatcher.go:393`, `internal/control/routing.go:242`, `internal/control/dispatcher_test.go:145` |
| `docs/planning/10` executor capabilities satisfy request constraints. | implemented | `internal/control/routing.go:311`, `internal/control/routing_test.go:163` |
| `docs/planning/11` registration carries supported ingress modes. | already existed | `api/proto/straw/v1/straw.proto:145`, `internal/control/worker_registry.go:553` |
| `docs/planning/11` registration validates capability scope. | implemented | `internal/control/worker_registry.go:608`, `internal/control/worker_registry_test.go:247` |
| `docs/planning/11` heartbeat-derived capability state includes advertised ingress modes. | already existed | `internal/control/worker_registry.go:341`, `internal/control/worker_registry.go:459` |
| `docs/planning/26` worker credential `allowed_capabilities.supported_ingress_modes`. | implemented | `internal/control/admin_handlers.go:687`, `internal/control/postgres_worker_credential_store.go:39`, `migrations/postgres/0007_worker_credential_capabilities.sql:2`, `internal/control/postgres_store_test.go:506` |
| `docs/planning/26` routing rule schema includes `match_conditions.ingress_type`. | already existed | `internal/control/config_admin_handlers.go:94`, `internal/control/postgres_config_store.go:305` |
| `docs/planning/26` worker credential `POST`, `GET`, and revoke endpoints preserve tenant scope. | implemented | `internal/control/admin_handlers.go:687`, `internal/control/admin_handlers.go:705`, `internal/control/admin_handlers_test.go:888` |

## Verification

```sh
go test ./internal/control -run 'TestDispatcherRoutesUsingRequestIngressType|TestRouting.*Ingress|TestRegisterIngressCapabilityOutOfScope|TestWorkerCredentialCreate.*Ingress|TestRoutingRuleRejectsUnknownIngressType|TestPostgresWorkerCredentialStoreSingleTenant'
make check
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
docker compose ps
```

Result:

- Focused tests: passed.
- `make check`: passed.
- Postgres-backed tests: attempted with the dedicated `straw_test` DSN but not exercised because localhost Postgres was unreachable (`connect: connection refused` on `localhost:5432`).
- Live compose verification: skipped because `docker compose ps` showed no running services.

## Reviewer Start Points

- `internal/control/dispatcher.go`
- `internal/control/routing.go`
- `internal/control/worker_registry.go`
- `internal/control/postgres_worker_credential_store.go`
- `internal/control/admin_handlers.go`
- `migrations/postgres/0007_worker_credential_capabilities.sql`

## Remaining Work

- None.

## Blockers

- None.

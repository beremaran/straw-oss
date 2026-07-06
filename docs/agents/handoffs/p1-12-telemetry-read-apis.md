# Handoff

Task: `docs/tasks/p1/12-telemetry-read-apis.md`

## Changed

- Added `internal/control/telemetry_read.go` with tenant-scoped telemetry handlers, ClickHouse read queries, public response schemas, cursor pagination, public aliasing, and query bound enforcement.
- Added `internal/control/telemetry_read_test.go` covering tenant scoping, topology redaction, query bounds, public alias filters, cursor binding, millisecond timestamps, and ClickHouse SQL limits.
- Wired telemetry routes into the built Control mux in `cmd/control/main.go` when ClickHouse is configured.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Telemetry APIs follow the task 11 spec. | VERIFIED | `internal/control/telemetry_read.go:204`, `cmd/control/main.go:716` | `TestTelemetryClickHouseSQLUsesTenantLimitsAndAliases`, `make check` |
| Tenant-facing responses do not expose worker IDs, session IDs, or selected executor unless aliased by the spec. | VERIFIED | `internal/control/telemetry_read.go:181`, `internal/control/telemetry_read.go:972`, `internal/control/telemetry_read.go:1080` | `TestTelemetryRequestsScopesTenantAndRedactsTopology`, `TestTelemetryWorkersFiltersPublicRefAndOmitsTopology` |
| Query limits protect ClickHouse. | VERIFIED | `internal/control/telemetry_read.go:331`, `internal/control/telemetry_read.go:365`, `internal/control/telemetry_read.go:671` | `TestTelemetryRejectsTenantOverrideAndWideWindow`, `TestTelemetryStoreErrorsMapToPublicResponses`, `make check` |

## Planning-Doc Coverage

Every in-phase field, endpoint, and behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `GET /api/v1/telemetry/requests` | implemented | `cmd/control/main.go:726`, `internal/control/telemetry_read.go:204` |
| `GET /api/v1/telemetry/requests/{request_id}` | implemented | `cmd/control/main.go:727`, `internal/control/telemetry_read.go:221` |
| `GET /api/v1/telemetry/workers` | implemented | `cmd/control/main.go:728`, `internal/control/telemetry_read.go:262` |
| `GET /api/v1/telemetry/audit` | implemented | `cmd/control/main.go:729`, `internal/control/telemetry_read.go:273` |
| Tenant ID derived from auth, not caller override | implemented | `internal/control/telemetry_read.go:444` |
| Telemetry read roles and requester detail access | implemented | `internal/control/telemetry_read.go:425`, `internal/control/telemetry_read_test.go:75` |
| Request list/detail public fields and attempt collapse/detail | implemented | `internal/control/telemetry_read.go:130`, `internal/control/telemetry_read.go:158`, `internal/control/telemetry_read.go:842` |
| Worker public schema and `worker_ref` alias/filter | implemented | `internal/control/telemetry_read.go:176`, `internal/control/telemetry_read.go:781`, `internal/control/telemetry_read.go:945` |
| Audit public schema and redacted stored values passthrough | implemented | `internal/control/telemetry_read.go:190`, `internal/control/telemetry_read.go:1067` |
| Query limits, max windows, ClickHouse execution/read limits | implemented | `internal/control/telemetry_read.go:331`, `internal/control/telemetry_read.go:365`, `internal/control/telemetry_read.go:671` |
| Cursor binding to tenant, endpoint, filter set, sort, window, timestamp, tie-break | implemented | `internal/control/telemetry_read.go:94`, `internal/control/telemetry_read.go:407`, `internal/control/telemetry_read.go:874` |
| Millisecond UTC RFC3339 output timestamps | implemented | `internal/control/telemetry_read.go:913`, `internal/control/telemetry_read_test.go:58` |
| Payload capture reads and dashboards | out of scope | `docs/tasks/p1/12-telemetry-read-apis.md` out-of-scope; dashboards owned by `docs/tasks/p1/13-observability-dashboards.md` |

## Verification

```sh
go test ./internal/control -run 'TestTelemetry'
make check
```

Result:

- `go test ./internal/control -run 'TestTelemetry'`: passed.
- `make check`: passed, including `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`.
- Postgres-backed tests: not exercised; diff does not touch Postgres files or migrations.
- Live compose verification: skipped; diff adds telemetry read handlers and ClickHouse query construction, not runtime request dispatch.

## Reviewer Start Points

- `internal/control/telemetry_read.go`
- `internal/control/telemetry_read_test.go`
- `cmd/control/main.go`

## Remaining Work

- None.

## Blockers

- None.

# Handoff

Task: `docs/tasks/p2/09-payload-capture-policy.md`

## Changed

- `internal/control/payload_capture.go` adds the tenant-scoped payload-capture policy model, disabled default, allowed decision validation, and in-memory test store.
- `internal/control/postgres_admission_config_store.go` and `migrations/postgres/0011_payload_capture_policy.sql` add the durable Postgres policy store/table.
- `internal/control/admin_handlers.go` and `cmd/control/main.go` wire `GET`/`PUT /api/v1/config/payload-capture` with RBAC, optimistic concurrency, audit, invalidation, and production store construction.
- `internal/control/handler.go`, `internal/control/stream_handler.go`, and `internal/control/request.go` make REST and REST-streaming request validation accept non-`none` `capture_hint` values only when the tenant policy allows them.
- `internal/control/admin_handlers_test.go` and `internal/control/handler_test.go` cover disabled defaults, roles, conflict, audit, REST validation, and stream validation.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Payload capture policy is explicit, tenant-scoped, and disabled by default. | VERIFIED | `internal/control/payload_capture.go:22`, `internal/control/payload_capture.go:50`, `migrations/postgres/0011_payload_capture_policy.sql:1`, `cmd/control/main.go:831`, `cmd/control/main.go:887` | `TestPayloadCapturePolicyDefaultsRolesAndConflict` |
| `capture_hint` values beyond `none` are accepted only when policy allows them. | VERIFIED | `internal/control/handler.go:102`, `internal/control/handler.go:104`, `internal/control/stream_handler.go:70`, `internal/control/stream_handler.go:72`, `internal/control/request.go:165`, `internal/control/payload_capture.go:87` | `TestHandlerCaptureHintAllowedByTenantPolicy`, `TestStreamHandlerCaptureHintAllowedByTenantPolicy` |
| No payload bytes are captured by this task. | VERIFIED | Existing metadata still records only sizes/decision at `internal/control/request_metadata.go:55` and `internal/control/request_metadata.go:196`; dispatcher still sends `PayloadCaptureDecision: none` at `internal/control/dispatcher.go:666` | Diff inspection plus `make check`; no payload-byte sink added |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Section 19 capture decisions `NONE`, `METADATA_ONLY`, `HEADERS`, `BODY_TRUNCATED`, `BODY_FULL` | implemented | `internal/control/payload_capture.go:13` |
| Payload capture off by default and tenant-admin enabled | implemented | `internal/control/payload_capture.go:50`, `migrations/postgres/0011_payload_capture_policy.sql:3`, `internal/control/admin_handlers.go:1131` |
| Capture boundary is storage tee only; no forwarded-byte mutation | already existed / preserved | No capture engine added; request validation only in `internal/control/handler.go:104` and `internal/control/stream_handler.go:72` |
| P2 `GET /api/v1/config/payload-capture` for `tenant_admin`, `operator`, `viewer` | implemented | `internal/control/admin_handlers.go:1110`, `cmd/control/main.go:887` |
| P2 `PUT /api/v1/config/payload-capture` for `tenant_admin` | implemented | `internal/control/admin_handlers.go:1131`, `cmd/control/main.go:888` |
| Shared config contract: `expected_config_version` and 409 conflict | implemented | `internal/control/admin_handlers.go:1144`, `internal/control/payload_capture.go:77`, `internal/control/admin_handlers_test.go:909` |
| P0/P1 capture hints other than `none` rejected unless policy permits | implemented | `internal/control/payload_capture.go:87`, `internal/control/handler_test.go:759`, `internal/control/handler_test.go:199` |
| No payload bytes captured by this policy task | implemented | No body storage path added; task 10 owns capture engine and task 11 owns storage |

## Verification

```sh
go test ./internal/control ./cmd/control
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
make check
```

Result:

- `go test ./internal/control ./cmd/control`: passed.
- `make check`: passed (`go test ./...` and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`, 0 issues).
- Postgres-backed tests: attempted against guarded `straw_test`, but local Postgres was unavailable on `localhost:5432` (`connection refused`). The guarded run did not complete.
- Live compose verification: skipped. This task changes config API and request validation, not payload capture execution; no live payload-byte behavior was added.

## Reviewer Start Points

- `internal/control/payload_capture.go`
- `internal/control/admin_handlers.go`
- `internal/control/handler.go`
- `internal/control/stream_handler.go`
- `migrations/postgres/0011_payload_capture_policy.sql`

## Remaining Work

- None. P2 capture engine and storage remain owned by `docs/tasks/p2/10-payload-capture-engine.md` and `docs/tasks/p2/11-payload-capture-storage.md`.

## Blockers

- None.

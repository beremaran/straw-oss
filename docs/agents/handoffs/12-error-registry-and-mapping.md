# Handoff

Task: `docs/tasks/p0/12-error-registry-and-mapping.md`

## Summary

The canonical `ErrorResponse` registry and executor-emittable `ErrorFrame` validation were
already largely in place from tasks 06/10/11 (`internal/control/errors.go`,
`internal/control/lifecycle.go`). This task closed the one real gap — the registry was missing
`header_injection_failed` (code 8) — and added a focused test suite that pins the full mapping
against `docs/planning/13`, `14`, `15`, and the `30-testing-matrix.md` error rows.

## Changed

- `internal/control/errors.go`
  - Added the missing canonical code `header_injection_failed` (`ErrorCode = 8`): the exported
    `HeaderInjectionFailed` constant, the `errorCodeHeaderInjectionFailed` wire string, and its
    `ErrorRegistry` entry (CLIENT / HTTP 400 / not retryable, per `docs/planning/14`). Before this
    the registry had 32 of the 33 canonical codes, so "every ErrorCode maps" was false.
- `internal/control/errors_test.go` (new)
  - `TestErrorRegistryCoversEveryProtoCode`: every non-`UNSPECIFIED` protobuf `ErrorCode` has
    exactly one registry entry with a matching numeric value, and entry count equals the proto
    enum size (no orphans, no gaps).
  - `TestErrorRegistryRows`: pins category/HTTP-status/retryable for all 33 rows of
    `docs/planning/14` (REST uses 404 for `route_no_match`, 499 for `cancelled`).
  - `TestErrorResponseIsPublicSafe`: `ErrorResponse` never exposes `worker_id`/`session_id`,
    `request_id` is always present, `details` values are strings.
  - `TestErrorResponseFallbackAndOmissions`: unknown codes fall back to `control_internal_error`;
    `retry_after_ms` and empty `details` are omitted.
  - `TestOriginStatusPassthroughIsNotErrorResponse`: origin 4xx/5xx statuses ride in the
    `SuccessResponse.status` envelope and never surface `category`/`code`/`retryable`
    (`docs/planning/15` origin passthrough; `30-testing-matrix.md` REST-outcome row).
  - `TestExecutorEmittableSetMatchesContract`: the emittable set equals the `docs/planning/13`
    list exactly; every out-of-set canonical code maps to `executor_internal_error` as a protocol
    violation.
  - `TestErrorCodeFromName`: round-trips code names including the newly added one.

## Verification

```sh
make check
```

Result: PASS. `go test ./...` all green; `golangci-lint run --max-issues-per-linter 0
--max-same-issues 0` reports `0 issues`.

Focused run:

```sh
go test ./internal/control/ -run 'Error|ExecutorEmittable|OriginStatus' -v
```

Result: PASS.

## Reviewer Start Points

- `internal/control/errors.go` — the `ErrorRegistry` map and the new code-8 entry.
- `internal/control/errors_test.go` — mapping/coverage/public-safety assertions.
- `internal/control/lifecycle.go` — `executorEmittableCodes` / `ValidateExecutorError` (pre-existing,
  now covered by `TestExecutorEmittableSetMatchesContract`).

## Error codes added or renamed

- Added `header_injection_failed` (`HeaderInjectionFailed`, code 8). No renames.

## Where future errors go

- Add the protobuf enum value in `api/proto/straw/v1/straw.proto` (regenerate),
  then add the exported `ErrorCode` constant, the `errorCode*` wire string, and an `ErrorRegistry`
  entry in `internal/control/errors.go`. `TestErrorRegistryCoversEveryProtoCode` and
  `TestErrorRegistryRows` will fail until the new code is fully mapped.
- If the code is executor-emittable, also add it to `executorEmittableCodes` in
  `internal/control/lifecycle.go` and extend `TestExecutorEmittableSetMatchesContract`.

## Remaining Work

- None for this task. Note the `ErrorResponse` struct does not yet carry `retry_after_ms` or
  `upstream_status` fields (`docs/planning/14` lists both as optional/omitted-when-not-applicable).
  Add `retry_after_ms` when rate-limit/quota task 13 needs to populate 429 responses, and
  `upstream_status` if/when a Straw error must echo an upstream code; both should use `omitempty`.
  [Update 2026-07-06 sweep: `retry_after_ms` landed with task 13 (`internal/control/errors.go:16`,
  populated on rate-limit denials via `internal/control/dispatcher.go:371`). `upstream_status` remains
  conditional ("if/when") and is not a gap.]

## Blockers

- None.

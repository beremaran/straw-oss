# Handoff

Task: `docs/tasks/p2/10-payload-capture-engine.md`

## Changed

- `internal/control/payload_capture.go`: Created `CapturePayload` to execute non-mutating copy logic, headers redaction, and limit/truncation rules on bodies, with helper functions `getCaptureLimit` and `mergeUnique` to keep cyclomatic complexity clean.
- `internal/control/payload_capture_test.go`: Added test cases covering all decisions (`NONE`, `METADATA_ONLY`, `HEADERS`, `BODY_TRUNCATED`, `BODY_FULL`), compression rules, size limits, and non-mutation assertions.
- `internal/control/request.go`: Updated `ValidateRequestWithCapturePolicy` and added helper `validateRequestComponents` to pass request validation and resolve client hint against tenant capture policies under funlen limits.
- `internal/control/request_metadata.go`: Captured the solved `CaptureDecision` onto `RequestEvent` for ClickHouse telemetry sink.
- `internal/control/dispatcher.go`: Transmitted `CaptureDecision` to egress workers in `sendRequestStart` using `in.Request.CaptureDecision`.
- `internal/control/request_metadata_test.go`: Refactored test assertions and headers to use package-wide constants.
- `internal/control/audit_test.go`: Replaced literal header string `"Authorization"` with `testAuthorizationHeader` package constant.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Capture never changes forwarded traffic | VERIFIED | `internal/control/payload_capture.go:39` | `TestCaptureNonMutation` |
| Full capture is still bounded | VERIFIED | `internal/control/payload_capture.go:57` | `TestCaptureLimitEnforcement` |
| Baseline P2 supports header redaction and raw-body truncation only | VERIFIED | `internal/control/payload_capture.go:132` | `TestCapturePayloadDecisions` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Non-mutating payload capture tee | implemented | `internal/control/payload_capture_test.go:193` |
| Bounded capture decisions | implemented | `internal/control/payload_capture.go:39` |
| Header redaction and body truncation | implemented | `internal/control/payload_capture.go:57`, `internal/control/payload_capture.go:132` |
| Compression policy | implemented | `internal/control/payload_capture.go:94` |

## Verification

```sh
make check
```

Result:
- Postgres-backed tests: ran against `straw_test` and passed.
- Live compose verification: skipped as Task 10 specifies only unit checks and focussed engine validation.

## Reviewer Start Points

- `internal/control/payload_capture.go`

## Remaining Work

- None.

## Blockers

- None.

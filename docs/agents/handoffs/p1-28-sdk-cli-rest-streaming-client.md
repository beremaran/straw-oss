# Handoff

Task: `docs/tasks/p1/28-sdk-cli-rest-streaming-client.md`

## Changed

- Added SDK streaming types and `Client.DoStream` for `POST /api/v1/requests:stream`.
- Added frame-by-frame binary parsing for metadata, body, trailers, end, and error frames.
- Added `straw request --stream`, writing upstream body bytes to stdout and stream metadata/trailers/end/error JSON to stderr.
- Updated stale task-09/task-10 handoff notes to point at this task's implemented SDK/CLI stream support.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| SDK calls `POST /api/v1/requests:stream` and parses metadata, body, trailers, end, and error frames. | VERIFIED | `sdk/stream.go:15`, `sdk/stream.go:35`, `sdk/stream.go:111`, `sdk/types.go:90` | `TestDoStreamParsesDocumentedFrames`, `TestDoStreamSurfacesPreMetadataAndPostMetadataErrors` |
| SDK does not buffer the full streamed body before exposing body chunks. | VERIFIED | `sdk/stream.go:85`, `sdk/stream.go:94`, `sdk/stream.go:115` | `TestDoStreamParsesDocumentedFrames` |
| Pre-metadata HTTP ErrorResponse and post-metadata error frames surface canonical error fields. | VERIFIED | `sdk/stream.go:52`, `sdk/stream.go:62`, `sdk/stream.go:69`, `sdk/stream.go:123`, `sdk/stream.go:170` | `TestDoStreamSurfacesPreMetadataAndPostMetadataErrors` |
| CLI exposes stream mode through existing `request`, sends the same request envelope, writes body bytes to stdout, and writes metadata/trailers/end/error to stderr. | VERIFIED | `internal/cli/cli.go:118`, `internal/cli/cli.go:143`, `internal/cli/cli.go:153`, `internal/cli/cli.go:195`, `internal/cli/cli.go:212`, `internal/cli/cli.go:221` | `TestRequestCommandStreamSeparatesBodyAndMetadata`, `TestRequestCommandStreamWritesErrorFrameToStderr` |
| Task-09 and task-10 handoffs no longer point stream client support only at completed server task 06; they name task 28 where relevant. | VERIFIED | `docs/agents/handoffs/p1-09-go-sdk.md:68`, `docs/agents/handoffs/p1-10-cli.md:69` | Diff inspection |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Public base path `/api/v1`. | already existed | `sdk/client.go:14`; existing CLI base URL/path handling in `internal/cli/cli.go` |
| P0 `POST /api/v1/requests` JSON request schema remains available. | already existed | `sdk/types.go:5`, `sdk/client.go:52`, `internal/cli/cli.go:157` |
| P1 `POST /api/v1/requests:stream`. | implemented | `sdk/stream.go:15`, `sdk/stream.go:35`, `internal/cli/cli.go:153` |
| Streaming content type `application/vnd.straw.request-stream.v1+binary`. | implemented | `sdk/stream.go:16`, `sdk/stream.go:41` |
| Frame header: 1-byte type plus 4-byte big-endian payload length. | implemented | `sdk/stream.go:89`, `sdk/stream.go:94`, `sdk/stream.go:96` |
| Metadata frame with `request_id`, upstream `status`, and response headers. | implemented | `sdk/types.go:106`, `sdk/stream.go:130`, `sdk/client_test.go:193` |
| Body frame with raw upstream response bytes. | implemented | `sdk/stream.go:115`, `sdk/client_test.go:194`, `sdk/client_test.go:195` |
| Trailers frame with response trailer headers. | implemented | `sdk/types.go:113`, `sdk/stream.go:144`, `sdk/client_test.go:196` |
| End frame with final timing. | implemented | `sdk/types.go:118`, `sdk/stream.go:157`, `sdk/client_test.go:197` |
| Error frame with public ErrorResponse JSON after metadata/body. | implemented | `sdk/stream.go:170`, `sdk/client_test.go:254`, `internal/cli/cli_test.go:120` |
| Pre-metadata failures return normal public ErrorResponse with canonical HTTP status. | implemented | `sdk/stream.go:52`, `sdk/stream.go:69`, `sdk/client_test.go:234` |
| Origin HTTP status remains passthrough metadata, not a Straw API error. | implemented | `sdk/types.go:106`, `sdk/client_test.go:193` |
| Metadata is emitted before body bytes. | implemented | `TestDoStreamParsesDocumentedFrames` validates metadata before body at `sdk/client_test.go:209` |
| Post-metadata error emits error frame and no end frame. | implemented | `sdk/client_test.go:250`, `internal/cli/cli_test.go:118` |
| Request-body validation still uses inline request body; BodyRef remains out of scope. | already existed / out of scope | SDK request body remains `sdk/types.go:24`; BodyRef is owned by P2 BodyRef tasks. |
| Redirect responses pass through as upstream responses. | already existed | Stream exposes upstream status in metadata rather than converting status to SDK error: `sdk/types.go:106` |
| Trailers are exposed for this public streaming contract. | implemented | `sdk/types.go:113`, `internal/cli/cli.go:201` |
| P1 SDK/CLI minimal surfaces. | implemented | `sdk/doc.go:5`, `internal/cli/cli.go:129`, `internal/cli/cli.go:790` |

## Verification

```sh
go test ./sdk ./internal/cli ./cmd/straw
make check
```

Result:

- `go test ./sdk ./internal/cli ./cmd/straw`: passed.
- `make check`: passed, including `go test ./...` and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`.
- Independent verification: passed, with all acceptance criteria marked VERIFIED.
- Postgres-backed tests: not exercised; diff does not touch Postgres files, stores, or migrations.
- Live compose verification: skipped; diff is SDK/CLI client-side streaming over an existing server endpoint and does not change Control/Egress runtime request handling.

## Reviewer Start Points

- `sdk/stream.go`
- `sdk/types.go`
- `sdk/client_test.go`
- `internal/cli/cli.go`
- `internal/cli/cli_test.go`

## Remaining Work

- None.

## Blockers

- None.

# Handoff

Task: `docs/tasks/p1/06-rest-streaming-endpoint.md`

## Changed

- `internal/control/handler_test.go`: added stream-specific request body limit coverage.
- Verified the existing `POST /api/v1/requests:stream` binary endpoint, frame writer, raw dispatcher reuse, and route wiring.
- Updated task status and stale handoff notes that pointed at task 06 as open server work.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `/api/v1/requests:stream` returns the resolved binary content type and frame layout exactly. | VERIFIED | `cmd/control/main.go:732`, `internal/control/stream_handler.go:17`, `internal/control/stream_handler.go:242` | `TestStreamHandlerWritesBinaryMetadataBeforeBodyAndTrailers` |
| Existing non-streaming REST remains unchanged. | VERIFIED | `cmd/control/main.go:731`, `internal/control/handler.go:54`, `internal/control/handler.go:124` | `TestHandlerValidRequest` |
| Required decision acceptance tests are implemented. | VERIFIED | `docs/planning/32-open-decisions.md:10`, `internal/control/handler_test.go:66`, `internal/control/handler_test.go:137`, `internal/control/handler_test.go:173`, `internal/control/handler_test.go:198`, `internal/control/handler_test.go:231` | `TestStreamHandlerWritesBinaryMetadataBeforeBodyAndTrailers`, `TestStreamHandlerWritesErrorFrameAfterPartialBody`, `TestStreamHandlerDoesNotApplyInlineResponseBodyLimit`, `TestStreamHandlerEnforcesRequestBodyLimit`, `TestStreamHandlerAuthAndRBAC`, `TestStreamHandlerClientCancellationReturnsExistingError` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Public base path `/api/v1`. | already existed | `cmd/control/main.go:731`, `cmd/control/main.go:732` |
| `POST /api/v1/requests` non-streaming REST transport remains synchronous JSON. | already existed / preserved | `cmd/control/main.go:731`, `internal/control/handler.go:54` |
| `POST /api/v1/requests:stream` exists for P1 REST streaming. | implemented | `cmd/control/main.go:732`, `internal/control/stream_handler.go:29` |
| Binary frame format: 1 byte frame type, 4 byte big-endian payload length, payload bytes. | implemented | `internal/control/stream_handler.go:242`, `internal/control/stream_handler.go:248` |
| Content type `application/vnd.straw.request-stream.v1+binary`. | implemented | `internal/control/stream_handler.go:18`, `internal/control/stream_handler.go:204` |
| Frame type 1 metadata with `request_id`, upstream `status`, and REST `HeaderPair` base64 headers. | implemented | `internal/control/stream_handler.go:132`, `internal/control/stream_handler.go:207` |
| Frame type 2 raw upstream response body bytes. | implemented | `internal/control/stream_handler.go:167`, `internal/control/stream_handler.go:172` |
| Frame type 3 trailers with REST `HeaderPair` base64 headers. | implemented | `internal/control/stream_handler.go:138`, `internal/control/stream_handler.go:186` |
| Frame type 4 end with final timing. | implemented | `internal/control/stream_handler.go:142`, `internal/control/stream_handler.go:215` |
| Frame type 5 public ErrorResponse JSON. | implemented | `internal/control/stream_handler.go:226` |
| Metadata frame is emitted before body bytes. | implemented | `internal/control/stream_handler.go:167`, `internal/control/stream_handler.go:199` |
| Pre-metadata Control failures return normal public ErrorResponse with canonical HTTP status. | implemented | `internal/control/stream_handler.go:45`, `internal/control/stream_handler.go:59`, `internal/control/stream_handler.go:71`, `internal/control/stream_handler.go:115` |
| Upstream/transport error after emitted metadata/body writes an error frame and no end frame. | implemented | `internal/control/stream_handler.go:108`, `internal/control/handler_test.go:137` |
| Request body validation still uses `request.max_inline_request_body_bytes`. | implemented | `internal/control/stream_handler.go:71`, `internal/control/handler_test.go:198` |
| `request.max_inline_response_body_bytes` does not cap streamed response bytes. | implemented | `internal/control/handler_test.go:173` |
| NATS stream ordering, response frame types, trailers, end, error, cancellation, and credit semantics are reused through the raw dispatcher. | implemented | `internal/control/stream_handler.go:91`, `internal/control/dispatcher.go:174`, `internal/control/dispatcher.go:776` |
| BodyRef remains out of scope. | out of scope | P2 BodyRef response-body mode is blocked in `docs/planning/32-open-decisions.md`; client support remains outside this task. |
| SDK/CLI stream client support. | out of scope | Owned by `docs/tasks/p1/28-sdk-cli-rest-streaming-client.md`. |

## Verification

```sh
go test ./internal/control -run 'TestStreamHandler|TestRequestHandler'
make check
```

Result:

- Focused REST streaming tests: passed.
- `make check`: passed.
- Postgres-backed tests: not exercised (diff does not touch Postgres files or migrations).
- Live compose verification: skipped because `docker compose ps --format json` showed no running services.

## Reviewer Start Points

- `internal/control/stream_handler.go`
- `internal/control/handler_test.go`
- `cmd/control/main.go`

## Remaining Work

- None.

## Blockers

- None.

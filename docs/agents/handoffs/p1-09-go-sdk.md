# Handoff

Task: `docs/tasks/p1/09-go-sdk.md`

## Changed

- Added the public Go SDK package at `github.com/beremaran/straw/v2/sdk`.
- Added typed request, response, timing, and canonical `ErrorResponse` envelopes for `POST /api/v1/requests`.
- Added `Client.Do` for authenticated P0 REST request submission, API error parsing, replayable defaults, and cancellation through `context.Context`.
- Added SDK tests for request encoding, canonical errors, replayable defaults, upstream status handling, cancellation, and absence of P2 BodyRef/stream/MITM surface.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| The SDK can submit P0 REST requests and parse canonical errors. | VERIFIED | `sdk/client.go:14`, `sdk/client.go:52`, `sdk/client.go:86`, `sdk/types.go:64` | `TestDoEncodesRequestAndDefaultsReplayable`, `TestDoParsesCanonicalErrorResponse` |
| Replayable defaults match Section 7 and Section 10. | VERIFIED | `sdk/types.go:90` | `TestReplayableDefaultsOnlySafeMethods` |
| No P2-only API surface is exposed. | VERIFIED | `sdk/doc.go:3`, `sdk/types.go:5`, `sdk/types.go:24` | `TestRequestExposesNoP2OnlyBodyRefSurface` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Public base path `/api/v1` and P0 `POST /api/v1/requests`. | implemented | `sdk/client.go:14`, `sdk/client.go:61` |
| Bearer API key on request transport. | implemented | `sdk/client.go:68` |
| JSON request envelope `method`, `url`, `headers`, `body`, `routing`, `fingerprint_profile`, `timeout_ms`, `replayable`, `capture_hint`. | implemented | `sdk/types.go:5` |
| Header order/duplicates and base64 header values. | implemented | `sdk/types.go:18`, `sdk/client_test.go:41` |
| P0 inline request body only. | implemented | `sdk/types.go:24` |
| Routing hints `tags`, `country`, `region`, `ip_type`, `sticky_session_id`. | implemented | `sdk/types.go:30` |
| Replayable defaults: GET, HEAD, OPTIONS true; POST, PUT, PATCH, DELETE false. | implemented | `sdk/types.go:90`, `sdk/client_test.go:62` |
| Success response envelope `request_id`, upstream `status`, `headers`, `body`, `timing`. | implemented | `sdk/types.go:39` |
| Clients inspect JSON envelope `status`, not outer API HTTP status, for upstream result. | implemented | `sdk/doc.go:12`, `sdk/types.go:39`, `sdk/client_test.go:118` |
| Straw system failures return public ErrorResponse, origin statuses do not. | implemented | `sdk/client.go:86`, `sdk/client_test.go:82`, `sdk/client_test.go:118` |
| Canonical ErrorResponse fields: lower-snake-case `category`, `code`, optional `timeout_type`, omitted zero `retry_after_ms`, string `details`, present `request_id`. | implemented | `sdk/types.go:64`, `sdk/client_test.go:82` |
| `body_too_large` details use `direction` and `limit_bytes` strings when returned by Control. | implemented | SDK preserves arbitrary string `details`: `sdk/types.go:73` |
| Origin 3xx/4xx/5xx passthrough as successful upstream responses. | implemented | `sdk/client_test.go:118` |
| Context cancellation. | implemented | `sdk/client.go:61`, `sdk/client_test.go:142` |
| `/api/v1/requests:stream`. | out of scope | Server endpoint completed by `docs/tasks/p1/06-rest-streaming-endpoint.md`; SDK/CLI client support is owned by `docs/tasks/p1/28-sdk-cli-rest-streaming-client.md`. |
| P2 BodyRef/MITM API surface. | out of scope | Excluded by `sdk/types.go:24`, `sdk/doc.go:3`; guarded by `TestRequestExposesNoP2OnlyBodyRefSurface`. |

## Verification

```sh
go test ./sdk
make check
```

Result:

- `go test ./sdk`: passed.
- `make check`: passed, including `go test ./...` and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped (diff adds SDK client code and does not touch the runtime request path).

## Reviewer Start Points

- `sdk/client.go`
- `sdk/types.go`
- `sdk/doc.go`
- `sdk/client_test.go`

## Remaining Work

- None for this task. The server endpoint is complete; SDK/CLI stream client support is owned by
  `docs/tasks/p1/28-sdk-cli-rest-streaming-client.md`.

## Blockers

- None.

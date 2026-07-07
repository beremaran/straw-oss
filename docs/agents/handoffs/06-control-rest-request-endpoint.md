# Handoff

Task: `docs/tasks/p0/06-control-rest-request-endpoint.md`

## Changed

- Added `internal/control/request.go` with request envelope types (`RequestEnvelope`, `HeaderPair`, `RequestBody`, `RoutingHints`), validated request representation (`ValidatedRequest`), and comprehensive validation:
  - Method validation: required, uppercase, CONNECT rejected, valid HTTP token chars.
  - URL validation: required, http/https scheme, fragments rejected, userinfo rejected, empty host rejected, IPv6 zone identifiers rejected.
  - Header validation: max 64 headers, max 64-byte name, RFC 7230 token validation, Host header rejected, CR/LF rejected, base64 values decoded, max 16384 aggregate bytes.
  - Body validation: inline_base64 only in P0, BodyRef rejected, decoded size enforced against `max_inline_request_body_bytes`.
  - Timeout validation: minimum 1000ms, maximum bounded by config `max_timeout_ms`.
  - Capture hint validation: must be absent or "none".
  - Unknown fields rejected via `json.Decoder.DisallowUnknownFields()`.
- Added `internal/control/errors.go` with `ErrorResponse` struct matching the canonical error registry JSON format, `ErrorRegistry` map covering all P0 error codes (auth_failure through cancelled), `ErrorResponseFromCode` helper, and `WriteError`/`WriteValidationError` response writers.
- Added `internal/control/handler.go` with `RequestHandler` implementing `http.Handler` for `POST /api/v1/requests`:
  - Validates method (POST only), reads and parses JSON request body.
  - Delegates to `ValidateRequest` for schema validation.
  - Returns `ErrorResponse` for validation failures with correct HTTP status codes.
  - Returned `SuccessResponse` envelope through the original stubbed execution path.
    [Update 2026-07-07 sweep: this is historical only; the live handler now dispatches through
    `internal/control/handler.go` -> `DefaultRequestDispatcher`, closed by
    `docs/tasks/p0/24-control-request-dispatch-pipeline.md`.]
- Added `internal/control/handler_test.go` with 29 tests covering:
  - Valid request with headers and body.
  - Missing method, CONNECT rejection, URL fragment/userinfo rejection.
  - Host header rejection, duplicate header preservation.
  - Body limit exceeded (413), capture hint rejection, non-POST method.
  - Unknown fields rejection, invalid method casing.
  - Timeout too low, error registry completeness, error response mapping.
  - Request validation direct tests: empty method, invalid URL scheme, BodyRef rejection, base64 header decoding, body data decoding, no body, timeout defaults.
- Updated `cmd/control/main.go` to wire the handler into the HTTP server with `http.ServeMux`.
- Updated `internal/config/config.go` to add `MaxTimeoutMs` field to `ControlRequestConfig` with default of 120s.

## Verification

```sh
go test ./internal/control -v -count=1
make check
```

Result:
- 29 handler/validation tests pass.
- `make check` passes (all packages compile and tests pass).

## Reviewer Start Points

- [internal/control/request.go](internal/control/request.go) — validation logic
- [internal/control/errors.go](internal/control/errors.go) — error registry
- [internal/control/handler.go](internal/control/handler.go) — HTTP handler
- [internal/control/handler_test.go](internal/control/handler_test.go) — test coverage

## Remaining Work

- **Authentication and RBAC** (task 07): API key validation, tenant derivation, role-based authorization.
- **Routing evaluation** (task 09): Route matching, hard client hints, pool selection.
- **Assignment and stream lifecycle** (task 10): NATS subscription, AssignRequest dispatch, frame streaming.
- **Egress outbound execution** (task 11): Worker-side request execution, response frame collection.
- **Rate limits and quotas** (task 13): Redis-backed admission checks.
- **Deny rules** (task 10): Destination host/CIDR/IP deny checks.

> RESOLVED 2026-07-05 (P0 audit): every follow-up above is now done — see the board rows in `docs/tasks/p0.md`:
> 07 (`p0.md:15`), 09 (`p0.md:17`), 10 (`p0.md:19`), 11 (`p0.md:20`), 13 (`p0.md:21`). The stubbed execution path is
> now the live dispatch pipeline (task 24, `p0.md:31`) — see `internal/control/dispatcher.go`.

## Blockers

- None.

## Notes

- Historical note: the task-06 handler originally stubbed the execution path. That is now resolved by
  `docs/tasks/p0/24-control-request-dispatch-pipeline.md`; `RequestHandler` calls the real dispatcher and records
  the actual response/error outcome.
- The handler correctly validates the P0 request shape: all invalid fields, CONNECT, URL fragments, userinfo, Host header, BodyRef, capture_hint, body limits, and timeout limits are rejected.
- Sample valid request:
  ```json
  {"method":"GET","url":"https://example.com/path?x=1","headers":[{"name":"Accept","value_base64":"dGV4dC9odG1s"}],"timeout_ms":30000}
  ```
- Sample invalid requests:
  ```json
  {"method":"CONNECT","url":"https://example.com"}            // rejected: CONNECT
  {"method":"GET","url":"https://user:pass@host/path"}        // rejected: userinfo
  {"method":"GET","url":"https://host/path#frag"}             // rejected: fragment
  {"method":"GET","url":"https://host","headers":[{"name":"Host","value_base64":"aG9zdA=="}]} // rejected: Host header
  ```

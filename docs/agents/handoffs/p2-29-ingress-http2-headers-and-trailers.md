# Handoff

Task: `docs/tasks/p2/29-ingress-http2-headers-and-trailers.md`

## Changed

- Added focused MITM HTTP/2 tests for pseudo-header normalization, colon-prefixed header rejection, and response trailer forwarding.
- Updated the shared raw proxy test dispatcher to work with real `http.ResponseWriter` instances and emit test trailers.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Ingress HTTP/2 pseudo-headers are normalized as specified by task 14. | VERIFIED | `internal/control/mitm_handler.go:128` and `internal/control/request.go:319` | `TestMITMHTTP2PseudoHeadersNormalizeToRequest` (`internal/control/mitm_handler_test.go:431`) |
| Unsafe custom colon-prefixed headers are rejected or stripped according to the spec. | VERIFIED | `internal/control/proxy_handler.go:169` and `internal/control/request.go:342` | `TestMITMRejectsColonPrefixedHeader` (`internal/control/mitm_handler_test.go:461`) |
| HTTP/2 trailers follow the task 14 / NATS `TrailersFrame` ordering contract. | VERIFIED | `internal/control/dispatcher.go:1005`, `internal/control/dispatcher.go:1508`, and `internal/control/stream_handler.go:187` | `TestMITMHTTP2ForwardsTrailersAfterBody` (`internal/control/mitm_handler_test.go:482`) |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| HTTP/2 `:method` maps to request method. | already existed | Go h2 server normalizes pseudo-headers into `http.Request`; asserted at `internal/control/mitm_handler_test.go:450`. |
| HTTP/2 `:scheme`, `:authority`, and `:path` map to canonical MITM target URL. | already existed | `validateMITMRequest` builds the validated URL from host plus request URI; asserted at `internal/control/mitm_handler_test.go:453`. |
| Colon-prefixed headers are not forwarded to dispatcher/egress. | implemented | Asserted absent at `internal/control/mitm_handler_test.go:456`; invalid custom colon header rejected at `internal/control/mitm_handler_test.go:461`. |
| Unsafe custom colon-prefixed headers are rejected or stripped. | already existed | Shared header validation rejects non-token names; test covers the MITM ingress path at `internal/control/mitm_handler_test.go:461`. |
| NATS `TrailersFrame` precedes terminal `EndFrame`. | already existed | Dispatcher handles trailers before terminal end; stream ordering proof remains in `internal/control/handler_test.go:120`. |
| HTTP/2 clients receive trailers rather than dropping them. | implemented | MITM h2 response trailer assertion at `internal/control/mitm_handler_test.go:482`. |
| Ingress upload flow-control and live compose HTTP/2 proof. | out of scope | Owned by `docs/tasks/p2/30-ingress-http2-upload-flow-control-and-live-proof.md`. |

## Verification

```sh
go test ./internal/control -run 'TestMITMHTTP2|TestMITMRejectsColonPrefixedHeader|TestStreamHandlerWritesBinaryMetadataBeforeBodyAndTrailers'
go test ./internal/control
make check
```

Result:

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped because task 30 owns the live HTTP/2 MITM proof.

## Reviewer Start Points

- `internal/control/mitm_handler_test.go`
- `internal/control/proxy_handler_test.go`

## Remaining Work

- None for task 29. Task 30 owns ingress HTTP/2 upload flow control and live compose proof.

## Blockers

- None.

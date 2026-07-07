# Handoff

Task: `docs/tasks/p2/25-ingress-http2-stream-semantics.md`

## Changed

- Added focused MITM HTTP/2 tests in `internal/control/mitm_handler_test.go` for per-stream request IDs, per-stream cancellation isolation, and connection-close cancellation fanout.
- No production code change was needed: `MITMHandler` already generates one request ID per decoded request, and the dispatcher already maps request-context cancellation to a NATS `CancelFrame`.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Concurrent HTTP/2 ingress streams each receive a unique `request_id`. | VERIFIED | `internal/control/mitm_handler.go:65` generates a request ID per decoded request. | `TestMITMHTTP2StreamsHaveUniqueRequestIDs` (`internal/control/mitm_handler_test.go:431`) |
| Cancellation of one HTTP/2 stream publishes a NATS `CancelFrame` for the matching request and does not cancel sibling streams. | VERIFIED | `internal/control/dispatcher.go:899` maps request context cancellation to `sendCancel`; `internal/control/dispatcher.go:1053` publishes the `CancelFrame`. | `TestMITMHTTP2StreamCancelIsIsolated` (`internal/control/mitm_handler_test.go:475`); existing NATS proof in `TestDispatcherRawCancellationPublishesCancelFrame` |
| A client HTTP/2 connection-level failure fans out cancellation to all active in-flight streams on that connection. | VERIFIED | `internal/control/mitm_handler.go:106` passes each request context into `DispatchRaw`; `internal/control/dispatcher.go:899` maps each cancelled request context to `sendCancel`. | `TestMITMHTTP2ConnectionCloseCancelsAllStreams` (`internal/control/mitm_handler_test.go:502`) |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/c-http2-semantics.md` Section 2: each ingress HTTP/2 stream maps 1:1 to a unique Straw `request_id`. | implemented | `internal/control/mitm_handler.go:65`; `internal/control/mitm_handler_test.go:431` |
| `docs/planning/c-http2-semantics.md` Section 3: client stream close/reset cancels only that request and sends a NATS `CancelFrame`. | implemented | `internal/control/dispatcher.go:899`; `internal/control/dispatcher.go:1053`; `internal/control/mitm_handler_test.go:475` |
| `docs/planning/c-http2-semantics.md` Section 7: inbound client HTTP/2 connection failure cancels all active request contexts for that connection. | implemented | `internal/control/mitm_handler_test.go:502`; `internal/control/dispatcher.go:899` |
| `docs/planning/c-http2-semantics.md` pseudo-header and trailer rows. | out of scope | Owned by `docs/tasks/p2/29-ingress-http2-headers-and-trailers.md`. |
| `docs/planning/c-http2-semantics.md` upload flow-control and live proof rows. | out of scope | Owned by `docs/tasks/p2/30-ingress-http2-upload-flow-control-and-live-proof.md`. |
| `docs/planning/12-nats-protocol.md` `CancelFrame` subject/protocol contract. | already existed | `internal/control/dispatcher.go:1053` |
| `docs/planning/17-mitm-design-p2.md` MITM TLS termination boundary. | already existed | `internal/control/mitm_connect_handler.go`; unchanged by this task. |
| `docs/planning/30-testing-matrix.md` HTTP/2 stream cancellation and connection-level error rows for this ingress slice. | implemented | `internal/control/mitm_handler_test.go:475`; `internal/control/mitm_handler_test.go:502` |

## Verification

```sh
go test ./internal/control -run 'TestMITMHTTP2'
go test ./internal/control -run 'TestMITMHTTP2|TestMITMHandler|TestMITMConnect'
go test ./internal/control
make check
```

Result:

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because task 25 requires focused Control/MITM HTTP/2 tests; task 30 owns the live compose HTTP/2 proof.

## Reviewer Start Points

- `internal/control/mitm_handler_test.go`
- `internal/control/mitm_handler.go`
- `internal/control/dispatcher.go`

## Remaining Work

- None for task 25. Header/trailer behavior remains owned by `docs/tasks/p2/29-ingress-http2-headers-and-trailers.md`; flow control plus live proof remains owned by `docs/tasks/p2/30-ingress-http2-upload-flow-control-and-live-proof.md`.

## Blockers

- None.

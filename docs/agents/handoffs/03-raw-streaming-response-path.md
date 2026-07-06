# Handoff

Task: `docs/tasks/p1/03-raw-streaming-response-path.md`

## Changed

- `internal/control/dispatcher.go`: added `DispatchRaw`, raw response frame handling, trailer forwarding, post-header error handling, client-cancel propagation, and download-credit replenishment after downstream writes.
- `internal/control/proxy_handler.go`: routes proxy requests through `DispatchRaw` when the dispatcher supports it, without changing P0 REST JSON behavior.
- `internal/egress/executor.go` and `internal/egress/loop.go`: stream response frames as the upstream body is read and gate egress `DataFrame` publication on NATS download credit.
- Focused tests in `internal/control/*_test.go` and `internal/egress/*_test.go` cover raw passthrough, large bodies, trailers, partial post-header errors, cancellation, and download-credit gating.
- Updated the task 02 handoff/task note that previously pointed at this task as open work.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Proxy modes can stream raw upstream responses without JSON envelopes. | VERIFIED | `cmd/control/main.go:646`, `internal/control/proxy_handler.go:74`, `internal/control/dispatcher.go:727`, `internal/control/dispatcher.go:1199` | `TestProxyHandlerUsesRawDispatcherWithoutJSONEnvelope`, `TestDispatcherRawResponseStreamsPastInlineLimit`, `TestExecutorStreamsResponseFramesBeforeUpstreamCompletes`, `TestRawResponseHeaderAndTrailerFiltering` |
| Backpressure prevents unbounded buffering. | VERIFIED | `internal/control/dispatcher.go:841`, `internal/control/dispatcher.go:862`, `internal/control/dispatcher.go:970`, `internal/egress/loop.go:281`, `internal/egress/loop.go:434` | `TestWorkerDownloadCreditGatesResponseData`, `make check` |
| Cancellation reaches the running request. | VERIFIED | `internal/control/proxy_handler.go:76`, `internal/control/dispatcher.go:743`, `internal/control/dispatcher.go:850`, `internal/egress/loop.go:401` | `TestDispatcherRawCancellationPublishesCancelFrame`, `TestWorkerCancelFrameDuringExecutionProducesCancelledFrame`, `TestProxyHandlerClientCancellationReachesRawDispatcher` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P0 REST JSON transport remains non-streaming. | already existed / preserved | `internal/control/handler.go` still uses `Dispatch`; proxy-only raw path is in `internal/control/proxy_handler.go:74`. |
| P1 proxy ingress may stream raw upstream responses. | implemented | `internal/control/proxy_handler.go:74`, `internal/control/dispatcher.go:727`. |
| `/api/v1/requests:stream` remains unimplemented. | out of scope | Owned by `docs/tasks/p1/06-rest-streaming-endpoint.md`. |
| Core NATS stream frames carry `ResponseStart`, `DataFrame`, `TrailersFrame`, `EndFrame`, `ErrorFrame`, and `CancelFrame`. | implemented | `internal/control/dispatcher.go:790`, `internal/egress/executor.go:178`, `internal/egress/executor.go:188`. |
| Byte-credit backpressure applies to response/download bytes. | implemented | `internal/control/dispatcher.go:841`, `internal/egress/loop.go:281`, `internal/egress/loop.go:434`. |
| Origin 3xx/4xx/5xx statuses pass through as upstream responses. | implemented | `internal/control/dispatcher.go:794`, `internal/control/dispatcher_test.go:248`. |
| Post-header upstream/internal failure closes the stream instead of rendering a second JSON error. | implemented | `internal/control/dispatcher.go:806`, `internal/control/proxy_handler.go:79`, `internal/control/dispatcher_test.go:397`. |
| Client cancellation sends a `CancelFrame`. | implemented | `internal/control/dispatcher.go:745`, `internal/control/dispatcher.go:850`, `internal/control/dispatcher_test.go:314`. |
| Allowed HTTP/1.1 trailers are forwarded; hop-by-hop/internal trailers are stripped. | implemented | `internal/control/dispatcher.go:802`, `internal/control/dispatcher.go:1212`, `internal/control/dispatcher.go:1224`, `internal/control/dispatcher_test.go:280`. |
| CONNECT tunneling remains separate. | out of scope | Owned by `docs/tasks/p1/05-raw-connect-tunnel.md`. |

## Verification

```sh
go test ./internal/egress
make check
```

Result:

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped because the local compose stack was not running and the default compose config does not enable/publish the P1 proxy listener; the live request path is covered by in-process NATS/egress tests.

## Reviewer Start Points

- `internal/control/dispatcher.go`
- `internal/control/proxy_handler.go`
- `internal/egress/loop.go`
- `internal/egress/executor.go`

## Remaining Work

- None.

## Blockers

- None.

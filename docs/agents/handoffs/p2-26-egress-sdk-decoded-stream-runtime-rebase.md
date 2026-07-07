# Handoff

Task: `docs/tasks/p2/26-egress-sdk-decoded-stream-runtime-rebase.md`

## Changed

- Added `sdk/egress` decoded assignment runtime: exact-session assignment subscription, c2e subscription flush before `AssignAck`, decoded request start/body read, e2c publish sequencing, cancellation, response credit, and executor error-frame publishing.
- Replaced `internal/egress` decoded loop ownership with SDK wrappers: `cmd/egress` calls `internal/egress.Run`, which constructs `sdk/egress.Worker`; `internal/egress.NewWorker` is a compatibility wrapper.
- Kept official outbound HTTP execution in `internal/egress.Executor`; only protocol/runtime machinery moved.
- Removed the old internal decoded loop and moved decoded runtime coverage to SDK tests.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `internal/egress` no longer owns decoded HTTP assignment subscription, decoded stream framing, cancellation, response credit, or executor error-frame protocol except as delegated compatibility wrappers. | VERIFIED | `cmd/egress/main.go:273`, `internal/egress/registration.go:66`, `internal/egress/worker.go:10`; old `internal/egress/loop.go` removed. | Independent verifier confirmed no remaining internal `runDecodedRequest`, `prepareRequestStream`, `waitForResult`, or `responseCreditGate`. |
| `sdk/egress` runtime tests prove decoded assignment accept/reject, subscriber flush before `AssignAck`, sequence handling, cancellation, response credit, and executor error frames. | VERIFIED | `sdk/egress/assignment.go:97`, `sdk/egress/assignment.go:192`, `sdk/egress/assignment.go:253`, `sdk/egress/assignment.go:313`, `sdk/egress/assignment.go:447`. | `TestSDKWorkerServeSubscribesAndFlushesAssignmentSubject`, `TestSDKAssignmentAdmissionAcceptsAndRejects`, `TestSDKStreamValidatorRejectsSequenceGapAndAcceptsDuplicate`, `TestSDKDecodedRuntimeStreamsResponseAndHonorsDownloadCredit`, `TestSDKDecodedRuntimeCancellationAndExecutorError`. |
| `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches. | VERIFIED | `sdk/egress/assignment.go:3`, `sdk/egress/stream.go:3`, `sdk/egress/runtime.go:3`. | `rg '"github.com/beremaran/straw/v2/internal/' sdk/egress` returned no matches. |
| Existing official executor tests still pass. | VERIFIED | `internal/egress/executor.go` remains the outbound execution owner; `internal/egress/executor_test.go` still covers official execution. | `go test ./sdk/... ./internal/egress ./internal/control ./cmd/egress`; `make check`. |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/05-component-boundaries.md`: SDK owns NATS registration, heartbeat, assignment handling, stream framing, and error protocol with a pluggable execution seam. | implemented | Registration/heartbeat were existing; decoded assignment and stream runtime now live in `sdk/egress/assignment.go:97`, `sdk/egress/assignment.go:192`, `sdk/egress/assignment.go:253`, and `sdk/egress/assignment.go:575`. |
| `docs/planning/05-component-boundaries.md`: official Egress Worker remains the reference implementation and performs outbound execution. | implemented | `internal/egress/registration.go:66` wires SDK runtime to `internal/egress.Executor`; `internal/egress/executor.go` remains the HTTP execution implementation. |
| `docs/planning/12-nats-protocol.md`: exact-session assignment subscription and no queue group. | implemented | `sdk/egress/assignment.go:97`; `TestSDKWorkerServeSubscribesAndFlushesAssignmentSubject`. |
| `docs/planning/12-nats-protocol.md`: executor subscribes and flushes c2e before accepted `AssignAck`. | implemented | `sdk/egress/assignment.go:192`; `TestSDKWorkerServeSubscribesAndFlushesAssignmentSubject`. |
| `docs/planning/12-nats-protocol.md`: decoded c2e `RequestStart`, inline `DataFrame`, stream sequence validation, duplicate/gap handling, and upload credit. | implemented | `sdk/egress/assignment.go:349`, `sdk/egress/stream.go:58`; `TestSDKStreamValidatorRejectsSequenceGapAndAcceptsDuplicate`. |
| `docs/planning/12-nats-protocol.md`: e2c publish sequencing and response/download credit. | implemented | `sdk/egress/assignment.go:313`, `sdk/egress/assignment.go:325`, `sdk/egress/assignment.go:498`; `TestSDKDecodedRuntimeStreamsResponseAndHonorsDownloadCredit`. |
| `docs/planning/12-nats-protocol.md`: cancellation frame cancels execution and emits `CancelledFrame` when cancellation wins. | implemented | `sdk/egress/assignment.go:447`, `sdk/egress/assignment.go:524`; `TestSDKDecodedRuntimeCancellationAndExecutorError`. |
| `docs/planning/12-nats-protocol.md` and `docs/planning/16-egress-execution.md`: executor error frames are emitted over e2c; error facts remain produced by official executor. | implemented | SDK publishes executor-returned error frames in `sdk/egress/assignment.go:286`; official errors remain built in `internal/egress/executor.go:1674`; `TestSDKDecodedRuntimeCancellationAndExecutorError`. |
| Raw CONNECT tunnel runtime. | out of scope | Owned by `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md`. |
| BodyRef request-body runtime rebase. | out of scope | SDK has an optional `BodyRefResolver` hook for compatibility; full rebase remains owned by `docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`. |

## Verification

```sh
go test ./sdk/... ./internal/egress ./internal/control ./cmd/egress
golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./sdk/egress ./internal/egress ./internal/control ./cmd/egress
rg '"github.com/beremaran/straw/v2/internal/' sdk/egress
make check
```

Result:

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because task 24 owns final SDK conformance and live compose verification after tasks 26, 27, 31, and 28.
- Grep/audit terms: `fake` hits are SDK/internal test doubles only; no runtime fake/stub/deferral was introduced.

## Reviewer Start Points

- `sdk/egress/assignment.go`
- `sdk/egress/assignment_test.go`
- `internal/egress/worker.go`
- `internal/egress/registration.go`
- `cmd/egress/main.go`

## Remaining Work

- Raw tunnel runtime movement remains owned by `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md`.
- BodyRef request-body runtime rebase remains owned by `docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`.
- Final SDK conformance and live compose verification remain owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md` after tasks 27, 31, and 28.

## Blockers

- None.

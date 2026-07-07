# Handoff

Task: `docs/tasks/p0/23-egress-assignment-execution-loop.md`

## Changed

- `internal/egress/loop.go` adds the live exact-session assignment subscriber, c2e subscription/flush-before-ack ordering, assignment reservation, request body stream validation, executor handoff, e2c publishing, cancellation handling, and worker drain behavior.
- `internal/egress/loop_test.go` covers accepted execution, capacity rejection, c2e credit exhaustion, cancellation-to-`CancelledFrame`, and shutdown drain over a real in-process NATS server.
- `internal/egress/runtime.go` starts the assignment loop after registration and reports live active/available capacity in heartbeats.
- `cmd/egress/main.go` constructs the real executor and passes it into the run loop.
- `internal/egress/executor.go` maps request cancellation to the canonical cancelled error so the loop can emit `CancelledFrame`.
- `internal/control/worker_nats_test.go` was updated for the new `egress.Run` signature.

## Verification

```sh
go test ./internal/egress/... -run TestWorker -v -timeout 60s
go test ./internal/egress ./internal/natsx ./cmd/... -timeout 120s
make check
```

Result: all passed; `make check` reported `0 issues` from golangci-lint.

## Reviewer Start Points

- `internal/egress/loop.go`
- `internal/egress/loop_test.go`
- `internal/egress/runtime.go`

## Remaining Work

- Control-side dispatch, response buffering, and Control-owned synthesized terminal outcomes remain deferred to `docs/tasks/p0/24-control-request-dispatch-pipeline.md`.
  [Update 2026-07-07 sweep: resolved by `docs/tasks/p0/24-control-request-dispatch-pipeline.md`; Control dispatch,
  response buffering, and terminal outcome mapping are now on the live request path.]
- Test-only `testutil.NewFakeNATSServer` is used in focused loop tests; no runtime fake, stub, or in-memory backend was added.

## Blockers

- None.

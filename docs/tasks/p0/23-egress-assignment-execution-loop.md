# 23 - Egress Assignment Execution Loop

Status: done

## Objective

Turn `cmd/egress` into a live worker that consumes exact-session assignments over NATS, executes outbound requests with
the existing executor, and streams terminal protocol frames back to Control.

## Required Planning Docs

- `docs/planning/16-egress-execution.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/09-canonical-request-lifecycle.md`
- `docs/planning/29-operational-behavior.md`

## Prerequisites

- Task 10 completed.
- Task 11 completed.
- Task 16 completed.
- Task 17 completed.

## Out of Scope

- Do not implement BodyRef transport.
- Do not implement provider adapters.
- Do not implement HTTP/2, MITM, or proxy ingress.
- Do not implement Control request dispatch (task 24).

## Expected Files

- Create or modify: `internal/egress` assignment execution loop.
- Modify: `cmd/egress/main.go`
- Test: focused egress assignment/stream tests.

## Steps

- [x] Read all required planning docs.
- [x] Subscribe on `straw.v1.executor.<worker_id>.<session_id>.assign` using the exact session from task 17; do not use
      queue groups for executor assignment subjects.
- [x] Validate and reserve assignments with the existing `EvaluateAssignment` logic, including capacity and deadline
      checks.
- [x] Before accepting, subscribe and flush the request-scoped `c2e` stream subject so Control cannot publish
      `RequestStart` before Egress is ready.
- [x] Reply with `AssignAck`, then process `RequestStart`, request body `DataFrame`s, credit, and cancel frames with
      the existing stream validation rules.
- [x] Execute accepted requests through `Executor.Execute`, preserving the DNS validation/dial-target invariant and P0
      HTTP/2 disabled behavior.
- [x] Publish `OutboundStartFrame`, `ResponseStart`, response `DataFrame`s, optional trailers, and exactly one terminal
      frame (`EndFrame`, `ErrorFrame`, or `CancelledFrame`) on the request-scoped `e2c` subject.
- [x] Enforce deadlines, upload/download/frame idle timeouts, cancellation, credit exhaustion, sequence/offset
      validation, and graceful drain behavior.
- [x] Add tests for assignment subscription ordering, rejected assignments, accepted execution, stream sequencing,
      credit/backpressure, terminal-frame uniqueness, cancellation, deadline expiry, and graceful drain.
- [x] Run focused egress assignment/stream tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused egress assignment/stream tests.
- `go test ./internal/egress ./internal/natsx ./cmd/...`
- `make check`

## Acceptance Criteria

- A live Egress worker consumes only its exact-session assignment subject and never relies on pool queue dispatch.
- Accepted assignments execute via the existing outbound executor and emit protocol frames over NATS.
- Every accepted request path ends with exactly one terminal frame or a synthesized terminal outcome after loss/deadline.
- Shutdown drains the assignment subscription and in-flight requests according to Section 29.

## Handoff Notes

- Document assignment subscription ordering, timeout behavior, and shutdown behavior.
- Note that Control-side dispatch and response buffering are deferred to `docs/tasks/p0/24-control-request-dispatch-pipeline.md`.

## Stop Conditions

- Stop before adding BodyRef, provider adapter, HTTP/2, MITM, or proxy behavior.
- Stop if a subscription would use queue-group dispatch for exact executor assignments.
- Stop if a deferral would have no owning task file.

# Handoff

Task: `docs/tasks/p0/10-assignment-and-stream-lifecycle.md`

## Changed

- `internal/control/lifecycle.go`: preserved fallback eligibility after pre-`RequestStart` reject/timeout/worker-loss terminals, kept total-deadline terminals non-replayable, and added timeout-type selection so simultaneous assignment/total deadlines choose total deadline.
- `internal/control/lifecycle_test.go`: added coverage for duplicate assignment acks, fallback after pre-start failure, replayable fallback boundaries, deadline no-fallback, unspecified executor error mapping, and simultaneous timeout priority.
- `internal/natsx/stream.go`: rejects `stream_seq=0`, rejects P0 `BodyRefFrame`, expires idle timeout at the boundary, and keeps shell/payload validation separated from sequencing.
- `internal/natsx/stream_test.go`: added zero-sequence, P0 BodyRef, and exact idle-boundary checks.
- `internal/natsx/nats_ordering_test.go`: added a small Core NATS ordering harness proving publish-before-subscribe/flush is lost and publish-after-flush is delivered.

## Verification

```sh
go test ./internal/control ./internal/egress ./internal/natsx
make check
```

Result:

- Focused lifecycle packages pass.
- `make check` passes: `go test ./...` passes and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reports `0 issues`.

## Reviewer Start Points

- `internal/control/lifecycle.go`
- `internal/natsx/stream.go`
- `internal/natsx/nats_ordering_test.go`
- `internal/egress/assignment.go`

## Remaining Work

- None for task 10.

## Blockers

- None.

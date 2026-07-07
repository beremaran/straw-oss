# Handoff

Task: `docs/tasks/p0/03-nats-connection-and-subjects.md`

## Changed

- Added shared NATS subject helpers in `internal/natsx` for registration, heartbeat, assignment, request stream, and terminal stream subjects.
- Added dot-free subject token validation so `worker_id`, `session_id`, and `request_id` reject unsafe characters before subject construction.
- Added startup payload validation against the configured NATS max payload safety margin and the configured frame/body limits.
- Wired `cmd/control` to validate NATS server configuration and payload limits after loading config.
- Wired `cmd/egress` to validate NATS server configuration after loading config.
- Expanded `internal/config` with the NATS and frame/body limit fields this startup validation needs.
- Added focused unit tests for exact subject formatting, unsafe token rejection, max payload validation, and server-list validation.

## Verification

```sh
go test ./internal/natsx
go test ./...
make check
```

Result:

- Passed.

## Reviewer Start Points

- [internal/natsx/natsx.go](/Users/beremaran/projects/straw/internal/natsx/natsx.go)
- [internal/natsx/natsx_test.go](/Users/beremaran/projects/straw/internal/natsx/natsx_test.go)
- [cmd/control/main.go](/Users/beremaran/projects/straw/cmd/control/main.go)
- [cmd/egress/main.go](/Users/beremaran/projects/straw/cmd/egress/main.go)

## Remaining Work

- Actual NATS client subscriptions and request flow are deferred to later tasks.
  [Update 2026-07-07 sweep: resolved — the binaries now hold live NATS connections via
  `docs/tasks/p0/16-nats-client-foundation.md`, worker registration/heartbeat subscriptions are wired by
  `docs/tasks/p0/17-worker-registration-heartbeat-over-nats.md`, egress assignment consumption is wired by
  `docs/tasks/p0/23-egress-assignment-execution-loop.md`, and Control request dispatch is wired by
  `docs/tasks/p0/24-control-request-dispatch-pipeline.md`.]

## Blockers

- None.

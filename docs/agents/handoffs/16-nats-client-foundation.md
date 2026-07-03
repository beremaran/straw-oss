# Handoff

Task: `docs/tasks/p0/16-nats-client-foundation.md`

## Changed

- Added `internal/natsx` connection helpers that connect with `DontRandomize`, reconnects, reconnect wait, ping interval, and max ping failure settings.
- Added live server `max_payload` verification against the control frame/body limits.
- Wired both `cmd/control` and `cmd/egress` to hold a real NATS connection at startup.
- Added fake-server tests for connect, reconnect, payload verification failure, and drain.
- Updated the task board and task spec status to `done`.

## Verification

```sh
make check
```

Result:

- Passed.

## Reviewer Start Points

- `/Users/beremaran/projects/straw/internal/natsx/connection.go`
- `/Users/beremaran/projects/straw/cmd/control/main.go`
- `/Users/beremaran/projects/straw/cmd/egress/main.go`

## Remaining Work

- None.

## Blockers

- None.

## Notes

- Reconnect settings are the config defaults from `internal/config`: `reconnect_attempts=10`, `reconnect_wait_ms=2000`,
  `ping_interval_ms=30000`, `max_ping_failures=3`.
- Shutdown uses `Drain()` instead of `Close()`.
- Subscription and publish wiring for registration, heartbeat, assignment, and stream frames is still deferred to
  tasks 17, 23, and 24.

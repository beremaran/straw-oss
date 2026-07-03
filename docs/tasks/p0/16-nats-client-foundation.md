# 16 - NATS Client Foundation

Status: done

## Objective

Add the real NATS client and connection lifecycle, and wire a live NATS connection into both `cmd/control` and
`cmd/egress` startup. This task authorizes adding the `github.com/nats-io/nats.go` dependency.

## Required Planning Docs

- `docs/planning/12-nats-protocol.md`
- `docs/planning/11-worker-discovery-and-health.md`
- `docs/planning/29-operational-behavior.md`
- `docs/planning/24-static-configuration.md`

## Prerequisites

- Task 03 completed.

## Out of Scope

- Do not implement any subscription or business message flow (registration, heartbeat, assignment, or stream
  frames) — that is tasks 17, 23, and 24.
- Do not add JetStream, durable streams, or queue-group dispatch for executor subjects.
- Do not implement worker or Control business logic beyond holding a connected client.

## Expected Files

- Create or modify: `internal/natsx` (connection wrapper)
- Modify: `cmd/control/main.go`
- Modify: `cmd/egress/main.go`
- Test: `internal/natsx/*_test.go`

## Steps

- [x] Read all required planning docs.
- [x] Add the `github.com/nats-io/nats.go` dependency to `go.mod`/`go.sum`.
- [x] Implement a connection wrapper in `internal/natsx` that dials the configured server list with reconnect and
      backoff, exposing the underlying `*nats.Conn` (or an interface over it) for later tasks to subscribe/publish on.
- [x] Implement connect-time verification of the live server's advertised `max_payload` against
      `control.transport.max_frame_data_bytes` (and the inline body limits), per the Section 12 rule
      (`max_frame_data_bytes <= nats.max_payload_bytes - 65536`). This is in addition to, not a replacement for, the
      existing config-vs-config `ValidateMaxPayload` check in task 03.
- [x] Implement graceful drain on shutdown (drain, not abrupt close) so in-flight request/reply exchanges are not cut
      off, consistent with the Control/Worker graceful shutdown sequences in Section 29.
- [x] Wire the connection wrapper into `cmd/control/main.go` startup so Control holds a live NATS connection before
      serving HTTP, and fails startup if the connection or max-payload verification fails.
- [x] Wire the connection wrapper into `cmd/egress/main.go` startup so Egress holds a live NATS connection, replacing
      the current discarded `_ = egress.NewExecutor(...)` dead end with a connection that later tasks (17, 23) can
      subscribe/publish on. This completes task 03's unchecked "wire connection setup" step.
- [x] Add tests for successful connect, reconnect/backoff behavior (using an embedded or fake NATS server), max
      payload verification failure at connect time, and graceful drain.
- [x] Run `go test ./internal/natsx ./cmd/...`.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/natsx`
- `go test ./cmd/...`
- `make check`

## Acceptance Criteria

- Both `cmd/control` and `cmd/egress` binaries construct and hold a real, connected NATS client at startup, not just
  validated config.
- Startup fails clearly if the live NATS server's max payload cannot satisfy the configured frame/body limits.
- Shutdown drains the connection instead of dropping it.
- No subscription or publish call for business subjects (registration, heartbeat, assignment, stream frames) exists
  yet.

## Handoff Notes

- Document the reconnect/backoff parameters used.
- Note that subscription/publish wiring for registration, heartbeat, assignment, and stream frames is deferred to
  tasks 17, 23, and 24.

## Stop Conditions

- Stop if a helper would allow pool queue dispatch.
- Stop before adding JetStream or durable stream behavior.
- Stop if a deferral would have no owning task file.

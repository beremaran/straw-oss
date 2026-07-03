# 17 - Worker Registration and Heartbeat over NATS

Status: done

## Objective

Turn worker discovery into a live wire protocol. Egress gets a real run loop: publish registration using the
existing builders in `internal/egress/registration.go`, then periodic heartbeats. Control subscribes on the
registration/heartbeat subjects (helpers in `internal/natsx/natsx.go`) using the `control` queue group and feeds
`WorkerRegistry.Register`/`Heartbeat`. Duplicate-session replacement and heartbeat-timeout state transitions must
work end to end over the wire, not just in-process against fakes.

## Required Planning Docs

- `docs/planning/11-worker-discovery-and-health.md`
- `docs/planning/12-nats-protocol.md`
- `docs/planning/29-operational-behavior.md`

## Prerequisites

- Task 08 completed.
- Task 16 completed.

## Out of Scope

- Do not implement assignment consumption or stream execution (task 23).
- Do not move worker runtime state to Redis (documented deferral in task 13's handoff; single-Control P0
  limitation).
- Do not implement admin disable/drain HTTP endpoints (already implemented by task 08; reuse as-is).

## Expected Files

- Create or modify: `internal/egress` (run loop, registration/heartbeat publish)
- Create or modify: `internal/control` (registration/heartbeat subscription handlers)
- Modify: `cmd/control/main.go`
- Modify: `cmd/egress/main.go`
- Test: registration/heartbeat-over-NATS integration tests.

## Steps

- [x] Read all required planning docs.
- [x] Give `cmd/egress` a real run loop: on startup, publish a `RegisterRequest` (built via
      `internal/egress/registration.go`) as a NATS request/reply call on `straw.v1.control.register`, block on the
      response, and exit non-zero on rejection.
- [x] After successful registration, start a periodic heartbeat loop on `straw.v1.control.heartbeat` using the
      `egress.heartbeat.interval_ms` default (5s) from Section 11, carrying health, active request count, capacity,
      and draining flag.
- [x] Handle OS shutdown signals (SIGINT/SIGTERM) in the Egress run loop: send a draining heartbeat, then exit per
      the Worker Graceful Shutdown sequence in Section 29.
- [x] In Control, subscribe to `straw.v1.control.register` and `straw.v1.control.heartbeat` using the `control` queue
      group (per the Section 12 subject table) and wire inbound messages into the existing
      `WorkerRegistry.Register`/`Heartbeat` methods.
- [x] Wire the Control subscription setup into `cmd/control/main.go` startup, using the live NATS connection from
      task 16.
- [x] Verify duplicate-session replacement (new registration for the same `worker_id` supersedes the old session
      after grace) and heartbeat-timeout transitions (unavailable at 15s, dead at 30s) work when driven over real
      NATS request/reply, not just direct `WorkerRegistry` calls.
- [x] Add an integration test that starts an embedded/fake NATS server, runs a real (or in-process-driven) Egress
      registration+heartbeat flow, and asserts the worker becomes visible via `WorkerRegistry`.
- [x] Add a test proving a locally started Egress worker appears in `GET /api/v1/admin/workers`.
- [x] Run focused registration/heartbeat integration tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused registration/heartbeat-over-NATS tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- A locally started Egress worker process (or its run loop invoked in-process) registers over NATS, appears in
  `GET /api/v1/admin/workers`, and shows heartbeat-derived runtime state without any direct in-process call into
  `WorkerRegistry` from the test.
- Duplicate-session replacement and heartbeat-timeout transitions work identically to the task 08 in-memory tests
  when driven over the wire.
- Egress shuts down cleanly on signal, sending a draining heartbeat first.

## Handoff Notes

- Document the run-loop signal handling and heartbeat cadence used.
- Note that assignment consumption (task 23) and Redis-backed worker runtime state remain deferred, and to which
  task file.

## Stop Conditions

- Stop if a deferral would have no owning task file.
- Stop before implementing assignment subject subscription or execution.

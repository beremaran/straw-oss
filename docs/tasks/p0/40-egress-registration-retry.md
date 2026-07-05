# 40 - Egress Registration Retry and Recovery

Status: done

## Objective

Make the egress worker survive Control being temporarily unavailable: retry registration with backoff instead of
exiting fatally, and recover when its session dies on the Control side (e.g. a Control restart wiping the in-memory
worker registry).

## Context (gap being closed)

Observed live on 2026-07-05 in the compose stack: restarting `control` and `egress` together stranded the worker.
`egress.Run` (`internal/egress/runtime.go`) calls `Register` exactly once and returns its error, so `cmd/egress`
exits with `egress run loop: request straw.v1.control.register: nats: no responders available for request` whenever
Control's NATS subscriptions are not up yet. Recovery is delegated entirely to the container restart policy's
backoff, leaving multi-minute windows where `GET /api/v1/admin/workers` is `[]` and every dispatch fails
`route_unavailable`; outside docker there is no recovery at all. Compose's `depends_on: service_started` masks this
on first boot only by racing in Control's favor.

The second half of the same gap: Control's worker registry is in-memory, so a Control restart forgets the session
while the worker keeps heartbeating it. Control ignores stale-session heartbeats (`TestHeartbeatStaleSessionIgnored`),
the worker never learns, and it stays invisible until manually restarted.

## Required Planning Docs

- `docs/planning/11-worker-discovery-and-health.md` (registration/heartbeat lifecycle)
- `docs/planning/12-nats-protocol.md` (register/heartbeat subjects and reply semantics)
- `docs/planning/29-operational-behavior.md` (outage behavior expectations)

## Prerequisites

- Task 17 completed (registration/heartbeat over NATS).
- Task 35 completed (replay protection: retried registrations must use fresh nonces/issued-at, not resend a stale
  signed payload).

## Out of Scope

- Control-side outage hardening, cooldowns, and in-flight loss semantics (owned by
  `docs/tasks/p1/17-worker-loss-and-nats-outage-hardening.md`).
- Persistent worker state in Control (no owning task; raise it if this task proves it necessary).

## Expected Files

- Modify: `internal/egress/runtime.go` (retry loop around `Register`; each attempt builds a fresh signed request).
- Modify: `cmd/egress/main.go` only if the run-loop contract changes.
- Test: `internal/egress` tests for register-retry (fail N times then succeed) and for backoff/ctx-cancel behavior.

## Steps

- [x] Read all required planning docs.
- [x] Wrap registration in a bounded-backoff retry loop (respecting ctx cancellation); `/readyz` stays false until
      a registration succeeds. Every attempt must be freshly signed (new nonce + issued-at) so replay protection
      (task 35) does not reject retries.
- [x] Decide the stale-session recovery mechanism and record the decision in the handoff. The cheapest correct
      option is preferred — e.g. Control replying to a stale-session heartbeat with a NACK the worker treats as
      "re-register", or the worker re-registering when heartbeats fail for a full unavailable window. If the chosen
      mechanism needs protocol additions beyond P0, implement only the retry loop here and record the stale-session
      half as a scope extension of `docs/tasks/p1/17-worker-loss-and-nats-outage-hardening.md` — do not leave it
      unowned. (Chosen: heartbeat NACK — Control already replies `Ok:false, unknown_worker_session`; the worker now
      treats any heartbeat rejection as session-lost and re-registers. No protocol additions needed.)
- [x] Add tests: registration succeeds after transient no-responder errors; retries stop on ctx cancel; retried
      attempts carry fresh nonces.
- [x] Verify live in compose: `docker compose restart control egress` (control coming up slower) must converge to a
      registered worker with no manual intervention, and a subsequent `POST /api/v1/requests` must round-trip.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/egress ./cmd/egress`
- Live compose restart check (documented in the handoff, not automated).
- `make check`

## Acceptance Criteria

- Egress no longer exits when Control is unavailable at startup; it registers as soon as Control is reachable.
- `docker compose restart control egress` converges to a ready, dispatchable worker without manual restarts.
- Retried registrations pass replay protection (fresh nonce per attempt, proven by test).
- The stale-session recovery half is either implemented or explicitly owned by the named P1 task.

## Handoff Notes

- Record the backoff parameters and the stale-session decision.

## Stop Conditions

- Stop if the recovery mechanism requires protobuf/protocol changes (that is a planning decision).
- Stop if a deferral would have no owning task file.

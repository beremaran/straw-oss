# Handoff

Task: `docs/tasks/p0/40-egress-registration-retry.md`

## Changed

- `internal/egress/runtime.go` — restructured `Run` into a register→serve loop:
  - `registerWithRetry` retries `Register` with bounded exponential backoff (1s initial, ×2, 30s cap, no jitter)
    until success or ctx cancel. Every attempt goes through `BuildRegisterRequest`, which already generates a fresh
    crypto/rand nonce and issued-at per call, so retries pass task-35 replay protection by construction.
  - `runSession` serves one registered session (assignment loop + heartbeats) and reports whether the session was
    lost. `runHeartbeatLoop` now returns `true` when Control NACKs a heartbeat (`Ok:false`, e.g.
    `unknown_worker_session` after a Control restart), which makes `Run` drain the dead session's assignment loop
    and re-register. Heartbeat transport errors are ignored as before; the next tick retries on the same session.
  - `/readyz` (the `ready` atomic) is false until registration succeeds, false again during the re-register window,
    and false once draining begins.
- `internal/egress/runtime_test.go` — new: retry succeeds after transient failures with distinct nonces per attempt;
  retry aborts from inside its backoff wait on ctx cancel; full `Run` re-registers after a heartbeat NACK, gets a new
  session, keeps heartbeating, and still shuts down cleanly on ctx cancel.
- No changes to `cmd/egress/main.go` (run-loop contract unchanged) and no protocol/protobuf changes.

## Decisions

- **Stale-session recovery: heartbeat NACK → re-register.** Control already replies `HeartbeatAck{Ok:false,
  Error:"unknown_worker_session"}` to stale sessions (`internal/control/worker_nats.go`); the worker previously
  discarded heartbeat errors. Zero protocol additions; the P1 scope extension escape hatch was not needed.
- **Backoff:** initial 1s, factor 2, cap 30s, retries indefinitely until ctx cancel. All registration errors retry,
  including explicit rejections — a Control-side internal error (e.g. Postgres briefly down during credential lookup)
  surfaces as a rejection, so treating rejections as fatal would reintroduce the outage-fragility this task removes.
  Marked with a `ponytail:` comment: classify permanent rejections as fatal and add jitter if fleets ever matter.

## Verification

```sh
make check                                  # gofmt + go test ./... + golangci-lint: 0 issues
go test -race -count=1 ./internal/egress    # ok
```

Live compose checks (2026-07-05, fresh volumes, `docker compose up -d --build`):

- `docker compose restart control egress`: egress came up first, logged
  `registration failed, retrying ... nats: no responders available for request` (previously the fatal exit), and
  converged to a `ready` worker with a new session in ~8s with no manual intervention.
- `docker compose restart control` only: running egress kept its old session, got the heartbeat NACK, logged
  `control rejected worker session, re-registering`, and re-registered a new session automatically — the exact
  stranding scenario from the 2026-07-05 audit.
- `POST /api/v1/requests` after recovery round-tripped `https://example.com/` with status 200.
- Note: the live check reset the dev compose volumes (`docker compose down -v`) because the previous session's
  bootstrap admin key was unknown; all seeded data is dev-only bootstrap state. The stack was stopped afterwards.

## Reviewer Start Points

- `internal/egress/runtime.go` (`Run`, `registerWithRetry`, `runSession`, `runHeartbeatLoop`)
- `internal/egress/runtime_test.go` (`TestRunReregistersWhenControlForgetsSession`)

## Remaining Work

- None. Nothing in this task is faked, stubbed, or deferred (tests use the pre-existing `testutil.NewFakeNATSServer`
  broker, consistent with the rest of the egress suite). Control-side outage hardening (cooldowns, in-flight loss
  semantics) remains owned by `docs/tasks/p1/17-worker-loss-and-nats-outage-hardening.md`, unchanged in scope.

## Blockers

- None.

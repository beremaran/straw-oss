# 45 - Redis-Backed Worker Runtime State

Status: done

## Objective

Move worker session, heartbeat/load, and cooldown runtime state from Control's process memory into the existing Redis
runtime-state tier, so Control restart or replica boundaries do not erase the worker availability state that
`docs/planning/21` assigns to Redis.

## Context (gap being closed)

The handoff sweep found a still-unowned P0-spec gap. `docs/planning/21-state-and-storage.md:62-76` says Redis stores
ephemeral runtime state with TTLs, including worker session/heartbeat/load state and cooldown state. Current code
still keeps that state only in `WorkerRegistry`'s in-process map: `internal/control/worker_registry.go:204-207`
documents it as the P0 in-process store and says Redis-backed TTL state is future work. Task 21 explicitly left
`WorkerRegistry` runtime state single-Control/in-memory (`docs/agents/handoffs/21-redis-wiring-and-config-invalidation.md:89-90`),
and task 40's Out of Scope recorded persistent worker state as having no owning task
(`docs/tasks/p0/40-egress-registration-retry.md:37-41`). This task is that owner.

## Required Planning Docs

- `docs/planning/11-worker-discovery-and-health.md` (runtime state model, heartbeats, duplicate sessions, cooldown)
- `docs/planning/21-state-and-storage.md` (Redis runtime-state ownership and TTL rule)
- `docs/planning/29-operational-behavior.md` (Redis unavailable behavior)
- `docs/planning/31-implementation-order.md` (Redis worker state item)

## Prerequisites

- Task 17 completed (worker registration/heartbeat over NATS). Done.
- Task 21 completed (Redis client and runtime-state wiring foundation). Done.
- Task 40 completed (egress retry and stale-session re-registration). Done.

## Out of Scope

- Do not make worker runtime state durable; Redis remains ephemeral and every key needs a TTL or documented lifecycle.
- Do not change global or tenant worker admin disable persistence; those already belong in Postgres.
- Do not build multi-Control durable request cancellation; `docs/tasks/p1/23-multi-control-durable-cancellation-state.md`
  owns that separate in-flight request gap.
- Do not add persistent request queues or automatic replay workflows.

## Expected Files

- Modify: `internal/control/worker_registry.go` (or an adjacent worker-runtime Redis store) to externalize session,
  heartbeat/load, duplicate-session, and cooldown state through Redis while preserving the current API.
- Modify: `cmd/control/main.go` to construct the Redis-backed worker runtime state in the built `cmd/control` binary.
- Test: `internal/control/worker_registry_test.go` and/or a new Redis-backed worker-runtime test for TTLs, restart-like
  reconstruction, cooldown, duplicate-session replacement, and Redis outage behavior.
- Test: `cmd/control` wiring test if construction is not covered through an existing startup test.

## Steps

- [x] Read all required planning docs.
- [x] Define the minimal Redis key shape and TTLs for worker sessions, heartbeat/load state, and cooldown state.
- [x] Keep the existing `WorkerRegistry` API stable; add the smallest store abstraction needed only if direct Redis
      calls would make tests or outage handling messy.
- [x] On registration and heartbeat, write the Redis runtime state with TTLs; on list/routing/candidate evaluation,
      read the current Redis-backed state or a bounded local snapshot per the Redis outage policy.
- [x] Preserve duplicate-session replacement and cooldown semantics from the current in-memory implementation.
- [x] Wire the Redis-backed runtime state in `cmd/control/main.go`; test-only registries may remain in-memory.
- [x] Add focused tests for TTL presence, restart-like reconstruction from Redis, cooldown retention within TTL,
      duplicate-session replacement, and Redis unavailable behavior.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- Worker session, heartbeat/load, and cooldown state are stored in Redis with TTLs or a documented short-lived lifecycle,
  proven by a Redis-backed test.
- A fresh `WorkerRegistry` built over the same Redis runtime state can list/evaluate a recently heartbeated worker
  without requiring the worker to re-register first, proven by a restart-like test.
- Duplicate-session replacement and cooldown behavior still match the current state machine, proven by tests.
- Redis unavailable behavior is explicit and matches `docs/planning/29`: worker availability uses a bounded local
  snapshot where possible and otherwise fails safe.
- `cmd/control/main.go` constructs the Redis-backed worker runtime state when Redis is configured.
- The task 21 handoff and task 40 Out of Scope notes name this task as the owner of the prior single-Control/in-memory
  deferral.

## Handoff Notes

- Record the Redis key prefixes, TTLs, and Redis-outage fallback behavior.
- Confirm whether the old in-memory implementation remains only as a test fallback or is deleted.
- Confirm the task 21 and task 40 stale notes now point at this task.

## Stop Conditions

- Stop if the required Redis outage behavior conflicts with `docs/planning/29`.
- Stop if preserving the current `WorkerRegistry` API would require a broad rewrite; propose a split first.
- Stop if a deferral would have no owning task file.

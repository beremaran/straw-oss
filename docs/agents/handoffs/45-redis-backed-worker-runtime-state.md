# Handoff

Task: `docs/tasks/p0/45-redis-backed-worker-runtime-state.md`

## Changed

- `internal/control/worker_runtime_redis.go`: added `RedisWorkerRuntimeStore` using `straw:worker-runtime:<worker_id>` keys. Each write stores the current session, superseded session, heartbeat/load fields, failure window, and cooldown timestamp with a TTL of `max(dead_timeout, cooldown_duration, cooldown_window)+1s`.
- `internal/control/worker_registry.go`: added the minimal `WorkerRuntimeStore` hook while preserving the existing registry API. Registration, heartbeat, and failure accounting persist runtime state; reads refresh from Redis and fall back to the local process snapshot on Redis errors.
- `cmd/control/main.go`: wires `NewRedisWorkerRuntimeStore(redisClient)` into the built Control binary.
- `internal/control/worker_registry_test.go`: added Redis-backed TTL/restart, duplicate-session/cooldown, and Redis-outage tests.
- `docs/agents/handoffs/21-redis-wiring-and-config-invalidation.md` and `docs/tasks/p0/40-egress-registration-retry.md`: stale deferrals now point at this task.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Worker session, heartbeat/load, and cooldown state are stored in Redis with TTLs or a documented short-lived lifecycle. | VERIFIED | `internal/control/worker_runtime_redis.go:38`, `internal/control/worker_runtime_redis.go:99`, `internal/control/worker_registry.go:1047` | `TestRedisWorkerRuntimeTTLAndRestartReconstruction` |
| A fresh `WorkerRegistry` built over the same Redis runtime state can list/evaluate a recently heartbeated worker without requiring the worker to re-register first. | VERIFIED | `internal/control/worker_registry.go:250`, `internal/control/worker_registry.go:520`, `internal/control/worker_runtime_redis.go:56` | `TestRedisWorkerRuntimeTTLAndRestartReconstruction` |
| Duplicate-session replacement and cooldown behavior still match the current state machine. | VERIFIED | `internal/control/worker_registry.go:332`, `internal/control/worker_registry.go:395`, `internal/control/worker_registry.go:464` | `TestRedisWorkerRuntimePreservesDuplicateAndCooldown` |
| Redis unavailable behavior is explicit and matches `docs/planning/29`: worker availability uses a bounded local snapshot where possible and otherwise fails safe. | VERIFIED | `internal/control/worker_registry.go:1025`, `internal/control/worker_registry.go:1044` | `TestWorkerRuntimeRedisOutageUsesLocalSnapshotThenFailsSafe` |
| `cmd/control/main.go` constructs the Redis-backed worker runtime state when Redis is configured. | VERIFIED | `cmd/control/main.go:121`, `cmd/control/main.go:123` | `go test ./cmd/control -count=1` |
| The task 21 handoff and task 40 Out of Scope notes name this task as the owner of the prior single-Control/in-memory deferral. | VERIFIED | `docs/agents/handoffs/21-redis-wiring-and-config-invalidation.md:89`, `docs/tasks/p0/40-egress-registration-retry.md:41` | File evidence |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Runtime state is derived from registration, heartbeat, duplicate-session handling, and cooldown. | implemented | `internal/control/worker_registry.go:332`, `internal/control/worker_registry.go:388`, `internal/control/worker_registry.go:440` |
| Heartbeat fields include health, active request count, max concurrency, available capacity, and draining. | implemented | `internal/control/worker_registry.go:404` |
| Control uses receive time for liveness and ignores stale sessions for routing while recognizing superseded sessions. | implemented | `internal/control/worker_registry.go:395`, `internal/control/worker_registry.go:407` |
| Duplicate registration replaces the current session and excludes the old session from routing. | implemented | `internal/control/worker_registry.go:332` |
| Cooldown is triggered by failures within the configured window and excludes workers until duration expires. | implemented | `internal/control/worker_registry.go:451`, `internal/control/worker_registry.go:464`, `internal/control/worker_registry.go:726` |
| Redis stores worker session/heartbeat/load state and cooldown state, with TTL on every key. | implemented | `internal/control/worker_runtime_redis.go:22`, `internal/control/worker_runtime_redis.go:48`, `internal/control/worker_runtime_redis.go:99` |
| Redis unavailable behavior uses local snapshot where possible and otherwise fails safe. | implemented | `internal/control/worker_registry.go:246`, `internal/control/worker_registry.go:1025`, `internal/control/worker_registry_test.go:708` |
| P0 Redis worker state item from implementation order. | implemented | `cmd/control/main.go:123` |
| Global and tenant worker admin disable persistence stays in Postgres. | out of scope | Task says not to change it; existing `rehydrateWorkerAdminState` remains in `cmd/control/main.go`. |

## Verification

```sh
go test ./internal/control ./cmd/...
go test ./internal/control -run 'TestRedisWorkerRuntimeTTLAndRestartReconstruction|TestRedisWorkerRuntimePreservesDuplicateAndCooldown|TestWorkerRuntimeRedisOutageUsesLocalSnapshotThenFailsSafe' -count=1 -v
make check
```

Result:

- Postgres-backed tests: not exercised; this diff does not touch Postgres files or migrations.
- Redis-backed tests: ran against the local compose Redis service and passed without skips.
- Live compose verification: completed by `docs/tasks/p0/46-live-compose-verification.md` on 2026-07-06. The full
  compose stack produced `straw:worker-runtime:<worker_id>` Redis JSON keys with session, heartbeat/load, pool, and
  capability fields plus TTLs. After stopping workers and restarting Control, `/api/v1/admin/workers` still listed
  the Redis-backed sessions without re-registration; the keys expired after their TTLs.

## Reviewer Start Points

- `internal/control/worker_runtime_redis.go`
- `internal/control/worker_registry.go`
- `internal/control/worker_registry_test.go`
- `cmd/control/main.go`

## Remaining Work

- None. The old in-memory map remains only as the registry's local Redis-outage snapshot and for test-only registries with no runtime store.

## Blockers

- None.

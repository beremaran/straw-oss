# Handoff

Task: `docs/tasks/p1/23-multi-control-durable-cancellation-state.md`

## Changed

- `docs/planning/32-open-decisions.md` — recorded the gate decision "P1 Multiple Concurrent Control Replicas —
  Resolved 2026-07-07": Straw supports multiple concurrent Control replicas sharing one request plane;
  cross-instance coordination lives in the existing Redis runtime-state tier (no new datastore) and is gated per
  deployment by `server.multi_control_enabled` (default off). This unblocks the task's Step-1 gate.
- `internal/config/config.go` — added `ControlServerConfig.MultiControlEnabled` (`multi_control_enabled`, default
  off), the per-deployment enablement flag.
- `internal/control/inflight.go` — added the `InFlightCrossInstance` interface and `SetCrossInstance`; `Register`/
  `Deregister` now take a `ctx` and advertise/clear ownership when a cross-instance collaborator is attached;
  `Cancel` now takes a `ctx` and, for a `request_id` not owned locally, resolves the owning tenant from the shared
  backend, applies the unchanged `AuthorizeAdminCancel`, and signals the owning instance; added unexported
  `cancelLocal` for the subscriber to apply a sibling-authorized cancel.
- `internal/control/inflight_redis.go` (new) — `RedisInFlightCoordinator` (ownership record
  `straw:inflight:<request_id>` → tenant_id with a TTL, cancel pub/sub on `straw:request:cancel`) and
  `RedisRequestCancelSubscriber` (mirrors the existing config-invalidation subscriber).
- `internal/control/dispatcher.go`, `tunnel_dispatcher.go` — pass the dispatch `ctx` to `Register`/`Deregister`.
- `internal/control/request_admin_handlers.go` — pass `r.Context()` to `InFlight.Cancel`.
- `cmd/control/main.go` — `wireInFlightRegistry(ctx, cfg, redisClient)` builds the registry, and only when
  `server.multi_control_enabled` is set attaches `RedisInFlightCoordinator` and starts
  `runRequestCancelSubscriber`; the registry is passed into `buildControlMux`.
- Tests: `inflight_test.go` (fake shared-backend cluster + four cross-instance tests; existing task-27 tests
  updated for the new `ctx` params), `inflight_liveredis_test.go` (new, gated on `STRAW_TEST_REDIS_URL`),
  `dispatcher_test.go` / `request_admin_handlers_test.go` (ctx params).
- `docs/agents/handoffs/27-admin-request-cancellation.md` — "Remaining Work" note updated: the gap is now closed by
  this task (p1/23).

## Chosen mechanism (why it stays in the Redis runtime-state tier)

`docs/planning/21` explicitly lists "short-lived in-flight request state" as Redis runtime state (with a mandatory
TTL) and exempts short-lived pub/sub from the TTL rule. So the design uses exactly those two sanctioned primitives:
a TTL-bounded ownership record (self-expires if an owner crashes) plus an ephemeral cancel pub/sub channel. No new
datastore, no persistent queue, no retry/replay (all out of scope / Future Work). The owning instance applies the
signal to its local `context.CancelFunc`, so the existing task-27 teardown (CancelFrame on the `c2e` subject +
cancelled terminal outcome) runs unchanged and exactly once.

## Acceptance Criteria Verdicts

Filled from the independent verifier (workflow step 12), not self-assessment. Verdict: **GO**, no bugs.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Cross-instance cancel tears down owner (two registries / shared backend) | VERIFIED | `internal/control/inflight.go` `Cancel` + `cancelViaCrossInstance`; `inflight_redis.go` | `TestInFlightRegistryCrossInstanceCancelReachesOwner`; live `TestRedisInFlightCoordinatorLive` |
| Local fast path cancels in-process, no backend touch | VERIFIED | `inflight.go` `Cancel` returns after `entry.cancel()` on local hit | `TestInFlightRegistryLocalFastPathSkipsBackend` (asserts lookups==0, signals==0) |
| Unknown request_id → existing not-found outcome | VERIFIED | `inflight.go` `Cancel` fall-through | `TestInFlightRegistryCrossInstanceUnknownReturnsNotFound` + existing unknown-request tests |
| `cmd/control` constructs cross-instance registry from redis client (binary wiring) | VERIFIED | `cmd/control/main.go` `run`→`wireInFlightRegistry`→`buildControlMux`; gated by `MultiControlEnabled`, default off | wiring traced in built binary by verifier |
| Task-27 handoff note names p1/23 as owner / closed | VERIFIED | `docs/agents/handoffs/27-admin-request-cancellation.md` (2026-07-07 update) | n/a |
| `make check` passes | VERIFIED | — | `make check` → 0 issues |

Authorization is unchanged from task 27: `AuthorizeAdminCancel(identity, tenantID)` on both paths; tenant-scoped
callers get `ErrInsufficientPermissions` for unknown/foreign requests (no existence disclosure), proven by
`TestInFlightRegistryCrossInstanceForeignTenantDenied`. No lock is held across a backend call (verified).

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `12` CancelFrame / `c2e` subject reused by cross-instance teardown | implemented (reused, not re-added) | owner cancels local ctx → `dispatcher.go` `sendCancel` publishes CancelFrame on `c2e` |
| `21` short-lived in-flight request state in Redis, with TTL | implemented | `inflight_redis.go` `straw:inflight:<id>` with `defaultInFlightRecordTTL` |
| `21` pub/sub as ephemeral (TTL-exempt) runtime signal | implemented | `inflight_redis.go` `straw:request:cancel` |
| `29` no new terminal outcomes, no duplicate teardown | implemented | single `entry.cancel()` on the owner; `signals==1` asserted |
| `32` committed multi-Control decision (the gate) | implemented | `docs/planning/32-open-decisions.md` (Resolved 2026-07-07) |

## Verification

```sh
make check
```

Result:

- `make check`: green (`go test ./...` all ok; `golangci-lint` 0 issues), confirmed independently by the verifier.
- Postgres-backed tests: not exercised — the diff touches no `postgres_*` files or `migrations/`.
- Live verification: the real `RedisInFlightCoordinator` + `RedisRequestCancelSubscriber` were driven against the
  compose Redis (`STRAW_TEST_REDIS_URL='redis://localhost:6379/0' go test ./internal/control -run
  TestRedisInFlightCoordinatorLive` → PASS): instance A (no local ownership) resolved the owner via a real Redis
  GET, published on the real cancel channel, and instance B's real subscriber applied `cancelLocal`. This closes
  the fake-backend gap. A full two-Control-replica compose run with a slow upstream was not performed (heavy
  infra); the two-registry-over-live-Redis test exercises the identical code path end to end.

## Reviewer Start Points

- `internal/control/inflight.go` (Cancel / cross-instance path)
- `internal/control/inflight_redis.go` (coordinator + subscriber)
- `cmd/control/main.go` `wireInFlightRegistry` (binary wiring + flag gate)

## Remaining Work

- None. Nothing is faked, stubbed, or deferred in the shipped code. The fake `fakeCluster` in `inflight_test.go` is
  test-only; the real Redis backend is implemented and live-verified.

## Blockers

- None. Work is committed (see the commit that includes this handoff).

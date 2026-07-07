# Handoff

Task: `docs/tasks/p0/09-routing-evaluation.md`

## Changed

- `internal/control/routing.go` (new): `Router.Evaluate` — tenant-scoped rule matching in ascending
  priority order, pool-policy-gated degraded-worker filtering, capability matching (tags/country/region/
  ip_type/ingress_type), least-loaded executor selection with round-robin tie break, and sticky-session
  pin/refresh/fallback using the canonical Redis key shape (`straw:sticky:<tenant_id>:<sticky_session_id>`)
  emulated in-process via `StickyStore` (TTL from the matched rule, refreshed on each use, no durable
  Redis client wired yet — swap is mechanical). `StaticRuleProvider`/`StaticPoolPolicyProvider` are the
  P0 in-memory snapshot/policy sources.
- `internal/control/worker_registry.go`: added `CandidatesForPool(tenantID, poolID)` returning eligible
  worker sessions (same admin/runtime exclusion precedence as `EligibleForTenant`) with the capability and
  load fields routing needs (tags/countries/regions/ip_types/ingress_modes, active/max/available capacity).
- `internal/control/routing_test.go` (new): priority order, tenant isolation, hard client hints, degraded
  pool policy (both allow/deny), no-match, unavailable (no eligible executor), sticky success, sticky
  failure (no fallback), sticky fallback + re-pin, disabled-rule skip, sticky TTL expiry.

## Verification

```sh
go test ./internal/control/... -run 'TestRouting|TestStickyStore' -v
make check
```

Result: all pass. `golangci-lint run ./internal/control/...` was also run; the two new files' only
remaining findings are the same `wsl_v5`/`goconst`/`gofumpt` style categories already present throughout
the pre-existing package (237 issues repo-wide before this change) and are not part of `make check`.

## Reviewer Start Points

- `internal/control/routing.go:125` (`Router.Evaluate`)
- `internal/control/routing.go:157` (`evaluateSticky`)
- `internal/control/worker_registry.go` (`CandidatesForPool`)

## Remaining Work

- No Postgres-backed routing-rule/pool-policy store yet — `StaticRuleProvider`/`StaticPoolPolicyProvider`
  are in-memory only, matching the existing P0 pattern (`InMemoryQuotaStore`, etc.). Wiring these into
  `cmd/control` and a config API surface is left to a later task.

  > RESOLVED 2026-07-05 (P0 audit): closed by tasks 19/24 (both done). Routing rules/pool policies are now assembled
  > from the Postgres-backed config snapshot, and the live dispatch pipeline calls `router.Evaluate` on the request
  > path — see `internal/control/dispatcher.go:290` (`route()` at `:281`, invoked from the main flow at `:183`).
- No Redis-backed `StickyStore` — in-process TTL map only, matching the P0 in-memory pattern used
  elsewhere. Key shape and TTL-refresh semantics already match the canonical Redis structure so the swap
  is mechanical.
  [Update 2026-07-06 sweep: resolved — `RedisStickyStore` exists (`internal/control/sticky_redis.go`),
  added by task 13 (rate limits/quotas/sticky over Redis).]
- Executor `AssignRequest` dispatch (actually sending the assignment over the NATS subject
  `Router.Evaluate` resolves) is task 10 (Assignment and stream lifecycle), out of scope here.
  [Update 2026-07-07 sweep: resolved by the live transport tasks — `docs/tasks/p0/23-egress-assignment-execution-loop.md`
  consumes assignment requests on Egress, and `docs/tasks/p0/24-control-request-dispatch-pipeline.md` sends
  assignments from Control on the request path.]

## Blockers

- None.

## Notes for future work

- **Tie-breaking**: least-loaded is `active_requests / max_concurrency` (ascending); ties are broken by a
  per-tenant+pool round-robin cursor over workers sorted by `worker_id` for determinism.
- **Ignored inputs (P0)**: `queue_depth` from heartbeats is not used in load ranking (only
  active/max/available capacity); `stable egress identity` from registration is not yet consulted for
  sticky pinning — sticky pins to `worker_id` only, which is the fallback described in
  docs/planning/10-routing-model.md ("If no stable identity exists, they may pin to executor session").
  Pinning to a stable egress identity when known is left for when Provider Adapters (P2) or an egress
  identity field is threaded through `PoolCandidate`.
- **Destination policy eligibility**: the task's "destination policy eligibility" filter step is not
  implemented — no destination/deny-list policy model exists yet in `internal/control` (that's task 12/
  egress policy territory). Routing here only applies tenant/pool/health/draining/disable/cooldown/
  capability filters.
  [Update 2026-07-07 sweep: the destination-policy model now exists and is wired by
  `docs/tasks/p0/22-control-destination-policy-resolution.md`,
  `docs/tasks/p0/24-control-request-dispatch-pipeline.md`, and
  `docs/tasks/p0/26-egress-destination-policy-precedence.md`. Routing itself remains worker/pool/capability
  selection; destination allow/deny policy is enforced before and during execution, not as a router-only filter.]

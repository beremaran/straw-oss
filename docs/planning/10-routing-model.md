## 10. Routing Model

Routes are tenant-scoped and evaluated from an immutable snapshot captured at request start.

### Rule Shape

Each rule has:

- `id`,
- `tenant_id`,
- `priority`,
- `enabled`,
- `match_conditions`,
- `target_pool_id`,
- `sticky_session_ttl_seconds`,
- `allow_sticky_fallback`,
- `config_version`.

P0 does not support nested `fallback_pool_ids`. Fallback is modeled by lower-priority rules and by Control selecting
another eligible executor only when replay/fallback rules allow it.

### Match Conditions

```json
{
  "tags": [
    "string"
  ],
  "country": "ISO-3166-alpha-2",
  "region": "string",
  "ip_type": "datacenter | residential | mobile | isp | unknown",
  "ingress_type": "rest | http_proxy | connect | mitm",
  "target_host": "example.com or *.example.com"
}
```

Rules are evaluated in ascending priority order. Any client-supplied hint is a hard constraint. Missing hints mean no
preference.

### Executor Selection

After a rule selects a pool, Control chooses the least-loaded healthy eligible executor in that pool. Round-robin is the
tie breaker.

An executor is eligible only if:

- tenant scope matches,
- pool scope matches,
- version is compatible,
- health is `ready` or `degraded` with pool policy `allow_degraded_workers=true`,
- it is not administratively disabled,
- it is not draining,
- it has available capacity,
- heartbeat freshness is within the availability threshold,
- it is not in cooldown,
- capabilities satisfy all request constraints.

Eligibility precedence is:

```text
global_disabled > tenant_disabled > dead > duplicate_replaced > draining > tenant_draining > cooldown > heartbeat_stale > health > capacity > capability
```

A higher-precedence exclusion reason should be reported in internal diagnostics. Public errors remain canonical public
codes.

### Sticky Sessions

Sticky sessions pin to a stable egress identity when available. If no stable identity exists, they may pin to executor
session. Sticky state is stored in Redis with tenant/rule TTL.

If the sticky target is unavailable:

- default: fail with `sticky_session_unavailable`,
- if `allow_sticky_fallback=true`: choose another eligible executor and update affinity.

Sticky fallback follows the same replay/fallback boundary as non-sticky fallback.

### Fallback and Replay

Control fallback is internal recovery before a final client-visible result. It is not SDK/client retry.

P0 fallback is allowed:

- after assignment reject before `RequestStart`,
- after assignment timeout before `RequestStart`,
- after executor loss before `RequestStart`,
- after `RequestStart` only if `replayable=true` and Control has not sent any client-visible success response.

Automatic replay defaults:

- `GET`, `HEAD`, `OPTIONS`: SDKs may default `replayable=true` before sending the request.
- `PUT`, `DELETE`, `POST`, `PATCH`: default `replayable=false`.

Control never silently changes `replayable=false` to `true`.

If no rule matches, return `route_no_match` with HTTP 421 for decoded proxy modes and HTTP 404 for REST transport. The
ErrorCode remains `route_no_match` in both cases.

If a rule matches but no eligible executor exists after permitted fallback, return `route_unavailable` with HTTP 503.

If all otherwise-eligible executors reject for capacity, return `executor_capacity_exhausted` with HTTP 503.

### Quota Accounting Reference

Request-count quota does not increment per-fallback attempt. See Section 9 (Canonical Request Lifecycle) for the timeout
hierarchy and Section 20 (Rate Limits and Quotas) for quota accounting semantics.

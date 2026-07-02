## 20. Rate Limits and Quotas

### Rate Limits

Rate limits are short-term admission controls.

Dimensions:

- tenant,
- tenant + API key,
- tenant + target host,
- tenant + IP type.

Algorithm: Redis sliding-window log using sorted sets.

Breaches return `rate_limit_exceeded` with HTTP 429 and `retry_after_ms` when computable.

### Quotas

Quotas are long-term volume controls.

Metrics:

- monthly request count,
- monthly bandwidth bytes.

P0 quota behavior is operational admission control, not billing-grade accounting. P0 uses:

- Redis fixed-window counters for fast admission,
- ClickHouse request/usage events for durable operational analytics,
- Postgres quota configuration,
- optional Postgres aggregate checkpoints if implemented during P0.

P0 must not claim exact durable billing accuracy. If Redis quota counters are lost, behavior follows the configured
quota fail policy. Reconciliation from ClickHouse events into Redis/Postgres aggregates is P2 unless explicitly added to
P0 as a tested implementation item.

### Bandwidth Accounting

P0 counts:

- accepted inline request bytes after Base64 decode,
- upstream response body bytes received by Control,
- protocol overhead excluded from tenant quota unless a deployment explicitly chooses transport-byte accounting.

If a request fails before outbound execution, request-count quota accounting is configurable:

- `count_on_admission=true`: count admitted attempts even if transport fails,
- `count_on_success=true`: count only successful upstream transport.

P0 default: `count_on_admission=true` for request count; bandwidth counted only for bytes actually transferred.

### Redis Failure Policy

Fail policy is explicit and configurable per tenant/system.

| Control             | Default                                                                    | Notes                                  |
|---------------------|----------------------------------------------------------------------------|----------------------------------------|
| Rate limits         | fail open for internal/dev, fail closed optional for production tenants    | Operator decision                      |
| Quotas              | fail closed for paid/abuse-sensitive tenants, fail open only if configured | Avoid unbounded usage                  |
| Sticky sessions     | degrade according to route policy                                          | May fail sticky requests               |
| Worker availability | use local snapshot for short TTL, then fail safe                           | Avoid routing to stale workers forever |

### Reconciliation Position

P0 tests must verify quota behavior under Redis failure according to configured policy. P0 does not need to repair lost
Redis counters unless a reconciliation job is explicitly implemented.

A billing-grade or near-billing-grade quota system requires a later reconciliation design defining:

- durable usage-event source,
- aggregation cadence,
- idempotency key,
- late-arriving event handling,
- correction policy for Redis hot counters,
- user-visible quota display semantics.

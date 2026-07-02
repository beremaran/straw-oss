## 21. State and Storage

### Postgres

Postgres stores durable control-plane state:

- tenants,
- platform users and tenant users,
- API keys,
- worker credentials,
- executor pools,
- routing rules,
- fingerprint profiles,
- injection policies,
- quota/rate-limit config,
- deny rules,
- payload-capture policy,
- worker admin disable state,
- config versions.

Postgres is the source of truth. Control is the only service that reads/writes it.

### Canonical P0 Postgres Model

P0 does not need every production index optimized, but it does need canonical tables, uniqueness rules, and versioning.

Required P0 tables:

| Table                    | Purpose                                                                 | Required constraints / notes                                       |
|--------------------------|-------------------------------------------------------------------------|--------------------------------------------------------------------|
| `tenants`                | Tenant boundary and status                                              | unique `id`; `status`; soft delete timestamp                       |
| `platform_users`         | Platform administrators                                                 | unique `id`; role includes `system_admin`                          |
| `tenant_users`           | Tenant-local users                                                      | unique `(tenant_id, user_id)`                                      |
| `api_keys`               | Tenant-scoped API keys                                                  | hashed secret; visible prefix; role; revoked timestamp             |
| `worker_credentials`     | Worker credential public keys and scopes                                | credential status; Ed25519 public key; scope JSON or child tables  |
| `executor_pools`         | Tenant-visible pools                                                    | unique `(tenant_id, id)`; executor type                            |
| `worker_admin_state`     | Durable worker disable state                                            | unique `(tenant_id, worker_id)` or global worker scope             |
| `routing_rules`          | Route priority and match conditions                                     | unique `(tenant_id, id)`; indexed `(tenant_id, priority)`          |
| `deny_rules`             | Host/CIDR/CNAME deny and allow overrides                                | normalized host/cidr columns where possible                        |
| `fingerprint_profiles`   | Allowed profile names and worker compatibility                          | unique `(tenant_id, name)` plus built-in global profiles           |
| `injection_policies`     | Ordered header operations                                               | unique `(tenant_id, id)`; bounded operation count                  |
| `rate_limit_configs`     | Rate-limit dimensions and limits                                        | unique `(tenant_id, dimension, key)`                               |
| `quota_configs`          | Monthly request/bandwidth limits and fail policy                        | unique `(tenant_id, quota_period)`                                 |
| `config_audit_source`    | Durable source record for config changes before ClickHouse async export | append-only; not a compliance-grade immutable audit log by itself  |
| `tenant_config_versions` | Monotonic version per tenant snapshot                                   | unique `tenant_id`; incremented transactionally with config writes |

All mutable tenant-scoped config resources include:

- `tenant_id`,
- `id`,
- `created_at`,
- `updated_at`,
- `config_version`,
- `enabled` or `status` where applicable.

Config writes are transactional. A successful write increments the affected tenant's snapshot version and writes an
audit source record in the same transaction.

### Redis

Redis stores ephemeral runtime state only. Every Redis key must have a TTL unless it is a short-lived pub/sub/version
coordination key whose lifecycle is otherwise documented.

Redis stores:

- route snapshot invalidation signals,
- latest tenant config-version hints,
- sticky session state,
- rate-limit counters,
- quota hot counters,
- worker session/heartbeat/load state,
- cooldown state,
- short-lived in-flight request state,
- P2 MITM cert cache/locks.

Redis data loss must not corrupt durable config. Redis loss may degrade availability decisions, sticky sessions, rate
limits, quotas, or certificate caches depending on explicit fail policy.

Redis eviction policy should not place all runtime state in one undifferentiated `volatile-lru` pool. Use logical DBs or
key prefixes with memory policies where deployment supports it. At minimum, quota/rate counters and worker availability
must not be evicted before best-effort cache data such as MITM cert cache.

### ClickHouse

ClickHouse stores append-heavy operational data. It is not the source of truth for config.

### Metadata Redaction Boundary

P0 writes request metadata, not payload capture. Metadata can still contain secrets. P0 therefore applies these storage
rules before writing logs or ClickHouse records:

- `target_host` is stored.
- URL userinfo is rejected before dispatch and never stored.
- `target_url` storage is sanitized by default: scheme, host, port, and path may be stored; query string is dropped
  unless tenant policy explicitly allows query storage.
- If query storage is disabled, Control may store a stable hash of the query string for correlation.
- `Authorization`, `Cookie`, `Proxy-Authorization`, and `Set-Cookie` are never stored in P0 logs or ClickHouse.
- Header names may be stored only in bounded diagnostic contexts; header values are not stored in P0 metadata.
- Public ErrorResponse details must not include secrets, worker IDs, session IDs, NATS subjects, credentials, or full
  unsanitized URLs.

Payload capture in P2 may store redacted headers/bodies according to explicit capture policy, but that is separate from
P0 metadata.

### Backup and DR

Postgres requires backup/restore outside Straw. Operators must configure managed backups or documented self-managed
backups. Straw does not provide built-in disaster recovery in Phase 1.

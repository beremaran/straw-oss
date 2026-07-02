## 25. Dynamic Configuration

Dynamic config is stored in Postgres and managed through APIs. Every dynamic resource has:

- `id`,
- `tenant_id` where applicable,
- `created_at`,
- `updated_at`,
- `config_version`,
- status or enabled flag where applicable.

Config writes are atomic. Updates include expected `config_version`; mismatch returns `conflict`.

### Snapshot Versioning

Each tenant has a monotonic `tenant_config_version`. Any successful tenant-scoped config write increments that version
in the same Postgres transaction as the resource change.

Control caches full tenant snapshots keyed by:

```text
(tenant_id, tenant_config_version)
```

In-flight requests continue using their captured snapshot even if config changes during execution.

### Invalidation

Config invalidation uses Redis pub/sub as an acceleration mechanism:

```text
straw:config:invalidate:<tenant_id>
```

The message includes the new `tenant_config_version`.

Redis pub/sub is not durable and may be missed. Therefore Control must also use at least one durable/sticky
version-check
mechanism:

- read-through version check on cache miss,
- periodic Postgres version poll,
- Redis version key with TTL refreshed on writes,
- forced Postgres version check for sensitive operations such as API key revocation and deny-rule changes.

P0 minimum:

- every config write updates Postgres version,
- Control publishes Redis invalidation after commit,
- Control stores the latest seen version per tenant,
- Control periodically checks Postgres for tenant versions,
- API key revocation and worker credential revocation force cache invalidation before returning success.

Stale snapshots are acceptable only for already-admitted in-flight requests, not for new requests after Control has
observed a newer tenant config version.

## 26. Config Management API Surface

Canonical durable config base path: `/api/v1/config`.

Canonical runtime admin base path: `/api/v1/admin`.

### Shared Config API Contract

All config list endpoints support:

- `limit` (default 50, max 200): page size.
- `offset` (default 0): zero-based offset for pagination.
- Sorting is always by `created_at` descending, then `id` ascending.

All config update/create endpoints use `expected_config_version` in the request body:

```json
{
  "expected_config_version": 7,
  "field": "value"
}
```

Version mismatch returns HTTP 409 `conflict` with `details.current_config_version` containing the current version.

Create endpoints are not inherently idempotent unless the endpoint explicitly supports client-supplied stable IDs or a
future idempotency-key mechanism. For P0, routing rules and executor pools may use client-supplied stable IDs; other
resources use server-generated UUIDs.

Delete endpoints are **soft-delete** for all resources: the resource status changes to `deleted` and `deleted_at` is
set. The resource remains queryable for audit purposes but is excluded from routing evaluation.

Selected resources (routing rules, executor pools) allow client-supplied stable IDs for idempotent creates. All other resources use server-generated UUIDs.

### Config Endpoints

| Phase | Method | Path                              | Role                                 | Purpose                   |
|-------|--------|-----------------------------------|--------------------------------------|---------------------------|
| P0    | POST   | `/tenants`                        | `system_admin`                       | Create tenant             |
| P0    | GET    | `/tenants`                        | `system_admin`                       | List tenants              |
| P0    | GET    | `/tenants/{id}`                   | `system_admin`, tenant roles         | Get visible tenant        |
| P0    | PUT    | `/tenants/{id}`                   | `system_admin`                       | Update tenant             |
| P0    | DELETE | `/tenants/{id}`                   | `system_admin`                       | Soft-delete tenant        |
| P0    | POST   | `/platform-api-keys`              | `system_admin`                       | Create platform API key   |
| P0    | GET    | `/platform-api-keys`              | `system_admin`                       | List platform API keys    |
| P0    | POST   | `/platform-api-keys/{id}/revoke`  | `system_admin`                       | Revoke platform API key   |
| P0    | POST   | `/api-keys`                       | `tenant_admin`                       | Create tenant API key     |
| P0    | GET    | `/api-keys`                       | `tenant_admin`, `operator`           | List tenant API keys      |
| P0    | POST   | `/api-keys/{id}/revoke`           | `tenant_admin`                       | Revoke tenant API key     |
| P0    | POST   | `/worker-credentials`             | `tenant_admin`                       | Create worker credential  |
| P0    | GET    | `/worker-credentials`             | `tenant_admin`                       | List worker credentials   |
| P0    | POST   | `/worker-credentials/{id}/revoke` | `tenant_admin`                       | Revoke worker credential  |
| P0    | POST   | `/executor-pools`                 | `tenant_admin`                       | Create pool               |
| P0    | GET    | `/executor-pools`                 | `tenant_admin`, `operator`, `viewer` | List pools                |
| P0    | PUT    | `/executor-pools/{id}`            | `tenant_admin`                       | Update pool               |
| P0    | DELETE | `/executor-pools/{id}`            | `tenant_admin`                       | Delete/disable pool       |
| P0    | POST   | `/routing-rules`                  | `tenant_admin`, `operator`           | Create route              |
| P0    | GET    | `/routing-rules`                  | `tenant_admin`, `operator`, `viewer` | List routes               |
| P0    | PUT    | `/routing-rules/{id}`             | `tenant_admin`, `operator`           | Update route              |
| P0    | DELETE | `/routing-rules/{id}`             | `tenant_admin`, `operator`           | Delete route              |
| P0    | GET    | `/fingerprint-profiles`           | `tenant_admin`, `operator`, `viewer` | List profiles             |
| P0    | POST   | `/injection-policies`             | `tenant_admin`, `operator`           | Create injection policy   |
| P0    | GET    | `/injection-policies`             | `tenant_admin`, `operator`, `viewer` | List injection policies   |
| P0    | PUT    | `/injection-policies/{id}`        | `tenant_admin`, `operator`           | Update injection policy   |
| P0    | DELETE | `/injection-policies/{id}`        | `tenant_admin`, `operator`           | Delete injection policy   |
| P0    | GET    | `/quotas`                         | `tenant_admin`, `operator`, `viewer` | Get quota config/usage    |
| P0    | PUT    | `/tenants/{id}/quotas`            | `system_admin`                       | Update tenant quotas      |
| P0    | GET    | `/rate-limits`                    | `tenant_admin`, `operator`, `viewer` | Get rate-limit config     |
| P0    | PUT    | `/rate-limits`                    | `tenant_admin`                       | Update rate limits        |
| P0    | POST   | `/deny-rules`                     | `tenant_admin`                       | Create deny rule          |
| P0    | GET    | `/deny-rules`                     | `tenant_admin`, `operator`, `viewer` | List deny rules           |
| P0    | PUT    | `/deny-rules/{id}`                | `tenant_admin`                       | Update deny rule          |
| P0    | DELETE | `/deny-rules/{id}`                | `tenant_admin`                       | Delete deny rule          |
| P0    | GET    | `/changes`                        | `tenant_admin`, `operator`, `viewer` | List config audit history |

Quota configs are platform-managed: only `system_admin` writes them, via the tenant-explicit
`/tenants/{id}/quotas` path (platform keys carry no tenant identity). Tenants retain read access through `/quotas`.
Rate limits remain tenant self-protection controls managed by `tenant_admin`, bounded by the optional
`rate_limit_ceiling` on the tenant record; the platform-binding volume protection is the quota.

`POST /worker-credentials` in P0 forces `tenant_scope` to the caller's tenant and rejects `allowed_pools` entries
referencing any other tenant. Multi-tenant worker credentials are a platform-scoped (`system_admin`) operation
deferred to P1.

P0 fingerprint profiles are built-in and seeded; there is no P0 write API for `fingerprint_profiles`. `chrome_120` is
the only enabled executable descriptor. Historical `firefox_121` and `safari_17` rows remain disabled/unavailable,
and `default` is a separate alias to `baseline`, never an executable profile. Tenant-authored profiles, if added, are
a P1 config surface.
| P1    | POST   | `/rollback`                       | `tenant_admin`                       | Roll back config          |
| P2    | GET    | `/payload-capture`                | `tenant_admin`, `operator`, `viewer` | Get capture policy        |
| P2    | PUT    | `/payload-capture`                | `tenant_admin`                       | Update capture policy     |

### Runtime Admin Endpoints

| Phase | Method | Path                                   | Role                       | Purpose                         |
|-------|--------|----------------------------------------|----------------------------|---------------------------------|
| P0    | POST   | `/workers/{worker_id}/disable`         | `system_admin`             | Globally disable worker         |
| P0    | POST   | `/workers/{worker_id}/enable`          | `system_admin`             | Globally enable worker          |
| P0    | POST   | `/workers/{worker_id}/drain`           | `system_admin`             | Globally drain worker           |
| P0    | POST   | `/workers/{worker_id}/undrain`         | `system_admin`             | Stop global drain               |
| P0    | POST   | `/workers/{worker_id}/tenant-disable`  | `tenant_admin`             | Disable worker for tenant       |
| P0    | POST   | `/workers/{worker_id}/tenant-enable`   | `tenant_admin`             | Enable worker for tenant        |
| P0    | POST   | `/workers/{worker_id}/tenant-drain`    | `tenant_admin`, `operator` | Drain worker for tenant         |
| P0    | POST   | `/workers/{worker_id}/tenant-undrain`  | `tenant_admin`, `operator` | Stop tenant drain               |
| P0    | GET    | `/workers`                             | `system_admin`, `tenant_admin`, `operator` | List workers and state |
| P0    | POST   | `/requests/{request_id}/cancel`        | `system_admin`, `tenant_admin`, `operator` | Cancel request        |

Global worker actions require a platform-scoped key. Tenant worker actions derive tenant identity from the tenant-scoped
API key and affect only that tenant's routing eligibility. A global disable always wins over tenant enable.

`GET /workers` with a platform-scoped key returns all registered workers with runtime, global admin, and per-tenant
admin state. With a tenant-scoped key it returns only workers eligible for that tenant (worker IDs are visible to
tenant admins by necessity, since tenant worker admin actions take a `worker_id`); `session_id` and NATS subjects are
never returned to tenant-scoped keys.

`POST /requests/{request_id}/cancel` with a tenant-scoped key requires that the in-flight request belong to the
caller's tenant; otherwise Control returns `insufficient_permissions` without confirming the request exists.
`system_admin` may cancel any request.

### P0 Config Resource Schemas

The following are minimal canonical P0 shapes. Implementations may add read-only computed fields but must not remove
these fields without a versioned API change.

#### Tenant

```json
{
  "id": "ten_...",
  "name": "Example Tenant",
  "status": "active | suspended | deleted",
  "default_timeout_ms": 60000,
  "max_timeout_ms": 300000,
  "metadata_query_storage": "drop | hash | store",
  "metadata_path_storage": "store | hash | drop",
  "rate_limit_ceiling": {
    "window_seconds": 60,
    "max_requests": 6000
  },
  "config_version": 1
}
```

`rate_limit_ceiling` is optional (`null` means no ceiling) and settable only by `system_admin` through
`PUT /tenants/{id}`. Tenant-managed rate-limit values that exceed the ceiling are rejected with `invalid_request`.

#### Platform API Key Create Request

```json
{
  "role": "system_admin"
}
```

#### Platform API Key Create Response

```json
{
  "id": "key_...",
  "scope_type": "platform",
  "tenant_id": null,
  "role": "system_admin",
  "prefix": "sk_live_abcd",
  "secret": "sk_live_abcd...",
  "created_at": "2026-07-02T00:00:00Z",
  "config_version": 1
}
```

The first platform key is bootstrapped through seed data, migration fixture, or environment bootstrap. All later platform
keys are managed through `/platform-api-keys` by `system_admin`.

#### Tenant API Key Create Request

```json
{
  "role": "requester | viewer | operator | tenant_admin"
}
```

#### Tenant API Key Create Response

```json
{
  "id": "key_...",
  "scope_type": "tenant",
  "tenant_id": "ten_...",
  "role": "requester | viewer | operator | tenant_admin",
  "prefix": "sk_live_abcd",
  "secret": "sk_live_abcd...",
  "created_at": "2026-07-02T00:00:00Z",
  "config_version": 1
}
```

The `secret` is returned only at creation time. Stored records contain only a secure hash and visible prefix.

#### API Key Read Response

```json
{
  "id": "key_...",
  "scope_type": "tenant",
  "tenant_id": "ten_...",
  "role": "requester",
  "prefix": "sk_live_abcd",
  "status": "active | revoked",
  "created_at": "2026-07-02T00:00:00Z",
  "revoked_at": null,
  "config_version": 1
}
```

#### Fingerprint Profile

```json
{
  "name": "chrome_120",
  "tenant_id": "ten_... or null for built-in",
  "enabled": true,
  "executor_type": "egress_worker",
  "profile_ref": "builtin:chrome_120",
  "min_worker_protocol_minor": 0,
  "config_version": 1
}
```

P0 fingerprint profiles are named presets. Control sends the resolved preset name to Egress. P0 does not expose
arbitrary JA3/JA4/TLS parameter authoring through the public config API. Availability is computed from enabled catalog
state plus currently eligible tenant-visible sessions advertising the exact signed capability; the read model reports
`supported`, `disabled`, `no_executable_definition`, or `no_active_capable_worker` without exposing worker/session IDs.

#### Routing Rule

```json
{
  "id": "route_default_us",
  "tenant_id": "ten_...",
  "priority": 100,
  "enabled": true,
  "match_conditions": {
    "tags": [
      "datacenter"
    ],
    "country": "US",
    "region": "us-west-1",
    "ip_type": "datacenter",
    "ingress_type": "rest",
    "target_host": "*.example.com"
  },
  "target_pool_id": "pool_us_west",
  "sticky_session_ttl_seconds": 3600,
  "allow_sticky_fallback": false,
  "config_version": 7
}
```

#### Executor Pool

```json
{
  "id": "pool_us_west",
  "tenant_id": "ten_...",
  "executor_type": "egress_worker",
  "enabled": true,
  "allowed_ip_types": [
    "datacenter"
  ],
  "allowed_countries": [
    "US"
  ],
  "allowed_regions": [
    "us-west-1"
  ],
  "tags": [
    "datacenter",
    "local"
  ],
  "allow_degraded_workers": false,
  "config_version": 3
}
```

#### Worker Credential

```json
{
  "id": "wcred_...",
  "tenant_scope": [
    "ten_..."
  ],
  "executor_type": "egress_worker",
  "allowed_pools": [
    { "tenant_id": "ten_...", "pool_id": "pool_us_west" }
  ],
  "allowed_capabilities": {
    "tags": [
      "datacenter"
    ],
    "countries": [
      "US"
    ],
    "regions": [
      "us-west-1"
    ],
    "ip_types": [
      "datacenter"
    ],
    "supported_ingress_modes": [
      "rest"
    ]
  },
  "public_key_ed25519_base64": "...",
  "status": "active",
  "config_version": 1
}
```

#### Deny Rule

```json
{
  "id": "deny_private_defaults",
  "tenant_id": "ten_...",
  "enabled": true,
  "type": "cidr | host | host_suffix | cname_suffix | metadata_ip | private_range",
  "value": "169.254.169.254/32",
  "action": "deny | allow_override",
  "reason": "metadata service blocked by default",
  "config_version": 2
}
```

#### Injection Policy

```json
{
  "id": "inject_default_headers",
  "tenant_id": "ten_...",
  "enabled": true,
  "operations": [
    {
      "op": "set | append | remove",
      "header_name": "User-Agent",
      "value_base64": "..."
    }
  ],
  "max_operations": 32,
  "config_version": 4
}
```

Operators may create or update injection policies only when all operations are non-sensitive. Operations that set or
append `Authorization` or `Cookie` require `tenant_admin`. `Host`, `Content-Length`, `Transfer-Encoding`, `Connection`,
`Proxy-Authorization`, and `X-Straw-*` remain denied unless another section explicitly allows them.

#### Rate Limit Config

```json
{
  "tenant_id": "ten_...",
  "limits": [
    {
      "dimension": "tenant | api_key | target_host | ip_type",
      "key": "*",
      "window_seconds": 60,
      "max_requests": 600,
      "fail_policy": "open | closed"
    }
  ],
  "config_version": 5
}
```

Rate-limit values are validated against the tenant's `rate_limit_ceiling` (see Tenant schema) when one is set.

#### Quota Config

```json
{
  "tenant_id": "ten_...",
  "period": "monthly",
  "max_requests": 1000000,
  "max_bandwidth_bytes": 1099511627776,
  "request_count_policy": "count_on_admission | count_on_success",
  "redis_fail_policy": "open | closed",
  "accuracy_level": "operational_admission_control",
  "config_version": 6
}
```

Quota configs are written only by `system_admin` through `PUT /tenants/{id}/quotas`.

All update requests include `expected_config_version`. Version mismatch returns `conflict`.

#### Config Audit Change

```json
{
  "id": "chg_...",
  "tenant_id": "ten_...",
  "actor_type": "api_key",
  "actor_id": "key_...",
  "config_type": "routing_rule",
  "resource_id": "route_default_us",
  "action": "create | update | delete | revoke",
  "config_version": 7,
  "field_path": "match_conditions.target_host",
  "old_value_json": "\"*.old.example\"",
  "new_value_json": "\"*.example.com\"",
  "created_at": "2026-07-02T00:00:00Z"
}
```

`actor_id` is the API key ID in P0. Secret fields are redacted before writing audit source records or ClickHouse events.

### P1 Config Resource Schemas

#### Rollback Request

```json
{
  "expected_config_version": 7,
  "target_config_version": 5,
  "reason": "restore previous route policy"
}
```

Rollback creates a new tenant config version; it does not reuse the target version number. Rollback restores only values
present in audit source records. Fields redacted as secrets cannot be restored by rollback and must be supplied again by
a tenant admin through the normal resource endpoint.

### P2 Config Resource Schemas

P2 adds `/payload-capture` schemas when payload capture implementation starts.

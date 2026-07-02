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

| Method | Path                              | Role                                 | Purpose                   |
|--------|-----------------------------------|--------------------------------------|---------------------------|
| POST   | `/tenants`                        | `system_admin`                       | Create tenant             |
| GET    | `/tenants`                        | `system_admin`                       | List tenants              |
| GET    | `/tenants/{id}`                   | `system_admin`, tenant roles         | Get visible tenant        |
| PUT    | `/tenants/{id}`                   | `system_admin`                       | Update tenant             |
| DELETE | `/tenants/{id}`                   | `system_admin`                       | Soft-delete tenant        |
| POST   | `/api-keys`                       | `tenant_admin`                       | Create API key            |
| GET    | `/api-keys`                       | `tenant_admin`, `operator`           | List API keys             |
| POST   | `/api-keys/{id}/revoke`           | `tenant_admin`                       | Revoke API key            |
| POST   | `/worker-credentials`             | `tenant_admin`                       | Create worker credential  |
| GET    | `/worker-credentials`             | `tenant_admin`                       | List worker credentials   |
| POST   | `/worker-credentials/{id}/revoke` | `tenant_admin`                       | Revoke worker credential  |
| POST   | `/executor-pools`                 | `tenant_admin`, `operator`           | Create pool               |
| GET    | `/executor-pools`                 | `tenant_admin`, `operator`, `viewer` | List pools                |
| PUT    | `/executor-pools/{id}`            | `tenant_admin`, `operator`           | Update pool               |
| DELETE | `/executor-pools/{id}`            | `tenant_admin`                       | Delete/disable pool       |
| POST   | `/routing-rules`                  | `tenant_admin`, `operator`           | Create route              |
| GET    | `/routing-rules`                  | `tenant_admin`, `operator`, `viewer` | List routes               |
| PUT    | `/routing-rules/{id}`             | `tenant_admin`, `operator`           | Update route              |
| DELETE | `/routing-rules/{id}`             | `tenant_admin`, `operator`           | Delete route              |
| GET    | `/fingerprint-profiles`           | `tenant_admin`, `operator`, `viewer` | List profiles             |
| POST   | `/injection-policies`             | `tenant_admin`, `operator`           | Create injection policy   |
| GET    | `/injection-policies`             | `tenant_admin`, `operator`, `viewer` | List injection policies   |
| PUT    | `/injection-policies/{id}`        | `tenant_admin`, `operator`           | Update injection policy   |
| DELETE | `/injection-policies/{id}`        | `tenant_admin`, `operator`           | Delete injection policy   |
| GET    | `/quotas`                         | `tenant_admin`, `operator`, `viewer` | Get quota config/usage    |
| PUT    | `/quotas`                         | `tenant_admin`                       | Update quotas             |
| GET    | `/rate-limits`                  | `tenant_admin`, `operator`, `viewer` | Get rate-limit config     |
| PUT    | `/rate-limits`                  | `tenant_admin`                       | Update rate limits        |
| POST   | `/deny-rules`                     | `tenant_admin`                       | Create deny rule          |
| GET    | `/deny-rules`                     | `tenant_admin`, `operator`, `viewer` | List deny rules           |
| PUT    | `/deny-rules/{id}`                | `tenant_admin`                       | Update deny rule          |
| DELETE | `/deny-rules/{id}`                | `tenant_admin`                       | Delete deny rule          |
| GET    | `/payload-capture`                | `tenant_admin`, `operator`, `viewer` | Get P2 capture policy     |
| PUT    | `/payload-capture`                | `tenant_admin`                       | Update P2 capture policy  |
| GET    | `/changes`                        | `tenant_admin`, `operator`, `viewer` | List config audit history |
| POST   | `/rollback`                       | `tenant_admin`                       | Roll back config          |

### Runtime Admin Endpoints

| Method | Path                            | Role                       | Purpose        |
|--------|---------------------------------|----------------------------|----------------|
| POST   | `/workers/{worker_id}/disable`  | `tenant_admin`             | Disable worker |
| POST   | `/workers/{worker_id}/enable`   | `tenant_admin`             | Enable worker  |
| POST   | `/workers/{worker_id}/drain`    | `tenant_admin`, `operator` | Drain worker   |
| POST   | `/workers/{worker_id}/undrain`  | `tenant_admin`, `operator` | Stop drain     |
| POST   | `/requests/{request_id}/cancel` | `tenant_admin`, `operator` | Cancel request |

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
  "config_version": 1
}
```

#### API Key Create Request

```json
{
  "role": "requester | viewer | operator | tenant_admin"
}
```

#### API Key Create Response

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
arbitrary JA3/JA4/TLS parameter authoring through the public config API.

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

All update requests include `expected_config_version`. Version mismatch returns `conflict`.

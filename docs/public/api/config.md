# Durable Configuration APIs

Straw keeps all configuration resources persistently in Postgres. When a resource is modified:
1. The tenant version is bumped in Postgres.
2. Invalidation messages are published via Redis pub/sub.
3. The Control plane cache refreshes, ensuring new policies take effect within milliseconds.

---

## Shared Request & Response Patterns

### Pagination (GET lists)
All configuration list endpoints support the following query parameters:
- `limit` (integer, default `50`, max `200`): number of records to return.
- `offset` (integer, default `0`): zero-based offset for pagination.
All list responses return a JSON array sorted by `created_at` descending, then `id` ascending.

### Versioned Updates (PUT / POST)
To prevent write collision (lost updates), all update and create endpoints accept or require:
- `expected_config_version` (integer) in the request body.
If the resource's current version does not match `expected_config_version`, Straw returns a `409 Conflict` status with the code `conflict` and details containing the current version.

### Soft-Deletion (DELETE)
DELETE requests on routing rules, pools, deny rules, and injection policies perform a soft-delete (status changes to `deleted` or record is omitted from routing, but retained in DB for change audit tracking). Deletions return a `204 No Content` status.

---

## 1. Tenants

### Create Tenant
- **Endpoint**: `POST /api/v1/config/tenants`
- **Role**: `system_admin`
- **Request Body**:
  ```json
  {
    "name": "Acme Corp"
  }
  ```
- **Response Status**: `201 Created`
- **Response Body**:
  ```json
  {
    "id": "ten_9a8f7b...",
    "name": "Acme Corp",
    "status": "active",
    "rate_limit_ceiling": null,
    "created_at": "2026-07-05T14:11:23Z",
    "config_version": 1
  }
  ```

### List Tenants
- **Endpoint**: `GET /api/v1/config/tenants`
- **Role**: `system_admin`

### Get Tenant
- **Endpoint**: `GET /api/v1/config/tenants/{id}`
- **Role**: `system_admin` or any role scoped to that tenant ID.

### Update Tenant
- **Endpoint**: `PUT /api/v1/config/tenants/{id}`
- **Role**: `system_admin`
- **Request Body**:
  ```json
  {
    "name": "Acme Corporation",
    "status": "active",
    "rate_limit_ceiling": {
      "window_seconds": 60,
      "max_requests": 5000
    },
    "expected_config_version": 1
  }
  ```
- **Response Body**: Returns updated Tenant object (`200 OK`).

### Soft-Delete Tenant
- **Endpoint**: `DELETE /api/v1/config/tenants/{id}`
- **Role**: `system_admin`
- **Response Body**: Returns Tenant object with status set to `"deleted"` (`200 OK`).

---

## 2. API Keys

### Create Platform Key
- **Endpoint**: `POST /api/v1/config/platform-api-keys`
- **Role**: `system_admin`
- **Request Body**:
  ```json
  {
    "role": "system_admin"
  }
  ```
- **Response Status**: `201 Created`
- **Response Body**: Returns key details including the plaintext `secret` (shown only once).

### Create Tenant Key
- **Endpoint**: `POST /api/v1/config/api-keys` (creates a key for the caller's tenant) or `POST /api/v1/config/tenants/{id}/api-keys` (system_admin bootstrapping first key for a tenant).
- **Role**: `tenant_admin` (for `/api-keys`) or `system_admin` (for `/tenants/{id}/api-keys`).
- **Request Body**:
  ```json
  {
    "role": "requester"
  }
  ```
- **Response Body**:
  ```json
  {
    "id": "6168de90-dc29-4fbc-bdd2-bb42c47ec2f9",
    "scope_type": "tenant",
    "tenant_id": "22222222-2222-4222-8222-222222222222",
    "role": "requester",
    "prefix": "sk_example_req",
    "secret": "sk_example_requester_secret_returned_once",
    "created_at": "2026-07-05T14:11:23Z",
    "config_version": 1
  }
  ```

### Revoke API Key
- **Endpoint**: `POST /api/v1/config/platform-api-keys/{id}/revoke` (platform) or `POST /api/v1/config/api-keys/{id}/revoke` (tenant).
- **Role**: `system_admin` (for platform) or `tenant_admin` (for tenant).
- **Response Body**: Returns the API key read object with status `"revoked"` (`200 OK`).

---

## 3. Worker Credentials

Used to register and authorize Egress worker instances.

### Create Worker Credential
- **Endpoint**: `POST /api/v1/config/worker-credentials`
- **Role**: `tenant_admin`
- **Request Body**:
  ```json
  {
    "executor_type": "egress",
    "allowed_pools": [
      {
        "tenant_id": "22222222-2222-4222-8222-222222222222",
        "pool_id": "dev-pool"
      }
    ],
    "public_key_ed25519_base64": "pRt97w+OVIQlr+uYeiECfFepc3WjylxQw/edkmcfprQ="
  }
  ```
- **Response Body**: Returns the created credential object with status `"active"` (`200 OK`).

### List Worker Credentials
- **Endpoint**: `GET /api/v1/config/worker-credentials`
- **Role**: `tenant_admin`
- **Response Body**: Returns worker credentials scoped to the caller's tenant. The response includes `id`, `tenant_scope`, `executor_type`, `allowed_pools`, `public_key_ed25519_base64`, `status`, and `config_version`.

### Revoke Worker Credential
- **Endpoint**: `POST /api/v1/config/worker-credentials/{id}/revoke`
- **Role**: `tenant_admin`
- **Response Body**: Returns the updated credential object with status `"revoked"` (`200 OK`).

---

## 4. Executor Pools

Groups of Egress workers mapped to specific routing rules.

### Create Executor Pool
- **Endpoint**: `POST /api/v1/config/executor-pools`
- **Role**: `tenant_admin`
- **Request Body**:
  ```json
  {
    "id": "pool_us_west",
    "executor_type": "egress",
    "tags": ["datacenter"],
    "enabled": true,
    "allow_degraded_workers": false,
    "allowed_ip_types": ["datacenter"],
    "allowed_countries": ["US"],
    "allowed_regions": ["us-west-1"]
  }
  ```
- **Response Body**: Returns created pool object (`200 OK`).

### List Executor Pools
- **Endpoint**: `GET /api/v1/config/executor-pools`
- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Response Body**: Returns the caller tenant's live executor pools.

### Update Executor Pool
- **Endpoint**: `PUT /api/v1/config/executor-pools/{id}`
- **Role**: `tenant_admin`
- **Request Body**: Same fields as create. The path supplies `id`; include `expected_config_version` for collision protection.
- **Response Body**: Returns updated pool object (`200 OK`).

### Delete Executor Pool
- **Endpoint**: `DELETE /api/v1/config/executor-pools/{id}`
- **Role**: `tenant_admin`
- **Response Status**: `204 No Content`

---

## 5. Routing Rules

Decides how outgoing client requests match against target Egress worker pools.

### Create Routing Rule
- **Endpoint**: `POST /api/v1/config/routing-rules`
- **Role**: `tenant_admin` or `operator`
- **Request Body**:
  ```json
  {
    "id": "route_default_us",
    "priority": 100,
    "enabled": true,
    "match_conditions": {
      "tags": ["datacenter"],
      "country": "US",
      "region": "us-west-1",
      "ip_type": "datacenter",
      "ingress_type": "rest",
      "target_host": "*.example.com"
    },
    "target_pool_id": "pool_us_west",
    "sticky_session_ttl_seconds": 3600,
    "allow_sticky_fallback": false
  }
  ```
- **Response Body**: Returns created routing rule (`200 OK`).

### List Routing Rules
- **Endpoint**: `GET /api/v1/config/routing-rules`
- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Response Body**: Returns the caller tenant's live routing rules.

### Update Routing Rule
- **Endpoint**: `PUT /api/v1/config/routing-rules/{id}`
- **Role**: `tenant_admin` or `operator`
- **Request Body**: Same fields as create. The path supplies `id`; `target_pool_id` is required.
- **Response Body**: Returns updated routing rule (`200 OK`).

### Delete Routing Rule
- **Endpoint**: `DELETE /api/v1/config/routing-rules/{id}`
- **Role**: `tenant_admin` or `operator`
- **Response Status**: `204 No Content`

---

## 6. Deny Rules

Enforces destination IP/Host block-lists.

### Create Deny Rule
- **Endpoint**: `POST /api/v1/config/deny-rules`
- **Role**: `tenant_admin`
- **Request Body**:
  ```json
  {
    "enabled": true,
    "type": "cidr | host | host_suffix | cname_suffix | metadata_ip | private_range",
    "value": "169.254.169.254/32",
    "action": "deny | allow_override",
    "reason": "Block cloud provider metadata"
  }
  ```
- **Response Body**: Returns created rule (`200 OK`).

### List Deny Rules
- **Endpoint**: `GET /api/v1/config/deny-rules`
- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Response Body**: Returns the caller tenant's live deny rules.

### Update Deny Rule
- **Endpoint**: `PUT /api/v1/config/deny-rules/{id}`
- **Role**: `tenant_admin`
- **Request Body**: Same fields as create. The path supplies `id`.
- **Response Body**: Returns updated deny rule (`200 OK`).

### Delete Deny Rule
- **Endpoint**: `DELETE /api/v1/config/deny-rules/{id}`
- **Role**: `tenant_admin`
- **Response Status**: `204 No Content`

---

## 7. Header Injection Policies

Modifies HTTP request headers prior to forwarding them to the destination.

### Create Injection Policy
- **Endpoint**: `POST /api/v1/config/injection-policies`
- **Role**: `tenant_admin` or `operator`
- **Request Body**:
  ```json
  {
    "enabled": true,
    "operations": [
      {
        "op": "set | append | remove",
        "header_name": "X-Client-Id",
        "value_base64": "MTIzNDU="
      }
    ]
  }
  ```
- **Response Body**: Returns created policy (`200 OK`).

### List Injection Policies
- **Endpoint**: `GET /api/v1/config/injection-policies`
- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Response Body**: Returns the caller tenant's live injection policies.

### Update Injection Policy
- **Endpoint**: `PUT /api/v1/config/injection-policies/{id}`
- **Role**: `tenant_admin` or `operator`
- **Request Body**: Same fields as create. The path supplies `id`.
- **Response Body**: Returns updated injection policy (`200 OK`).

### Delete Injection Policy
- **Endpoint**: `DELETE /api/v1/config/injection-policies/{id}`
- **Role**: `tenant_admin` or `operator`
- **Response Status**: `204 No Content`

> [!CAUTION]
> Operators can only inject non-sensitive headers. Injecting or appending credentials like `Authorization` or `Cookie` strictly requires the `tenant_admin` role. Custom values for headers like `Host`, `Connection`, and `X-Straw-*` are strictly blocked.

---

## 8. Quotas & Rate Limits

### Get Tenant Quotas
- **Endpoint**: `GET /api/v1/config/quotas`
- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Response Body**:
  ```json
  {
    "tenant_id": "22222222-2222-4222-8222-222222222222",
    "period": "monthly",
    "max_requests": 1000000,
    "max_bandwidth_bytes": 1099511627776,
    "request_count_policy": "count_on_admission",
    "redis_fail_policy": "open",
    "config_version": 1
  }
  ```

### Update Tenant Quota (Platform Operation)
- **Endpoint**: `PUT /api/v1/config/tenants/{id}/quotas`
- **Role**: `system_admin`
- **Request Body**:
  ```json
  {
    "period": "monthly",
    "max_requests": 2000000,
    "max_bandwidth_bytes": 2199023255552,
    "request_count_policy": "count_on_admission",
    "redis_fail_policy": "open",
    "expected_config_version": 1
  }
  ```
- **Response Body**: Returns updated Quota details (`200 OK`).

### Get Tenant Rate Limits
- **Endpoint**: `GET /api/v1/config/rate-limits`
- **Role**: `tenant_admin`, `operator`, or `viewer`

### Update Tenant Rate Limits
- **Endpoint**: `PUT /api/v1/config/rate-limits`
- **Role**: `tenant_admin`
- **Request Body**:
  ```json
  {
    "expected_config_version": 1,
    "limits": [
      {
        "dimension": "tenant | api_key | target_host | ip_type",
        "key": "*",
        "window_seconds": 60,
        "max_requests": 600,
        "fail_policy": "open | closed"
      }
    ]
  }
  ```
- **Response Body**: Returns updated RateLimitConfig (`200 OK`).

> [!WARNING]
> Values configured in tenant rate limits are validated against the tenant's global `rate_limit_ceiling` configured on the tenant record by the platform `system_admin`. If limits exceed the ceiling, Control rejects the change with an `invalid_request` error.

---

## 9. Fingerprint Profiles (Read-Only)

Fingerprint profiles are built-in presets seeded in the database. The public API exposes a read-only list; there is no write endpoint.

### List Profiles
- **Endpoint**: `GET /api/v1/config/fingerprint-profiles`
- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Response Body**:
  ```json
  [
    {
      "name": "chrome_120",
      "scope_type": "built_in",
      "supported_by_worker": true,
      "enabled": true,
      "config_version": 1
    }
  ]
  ```

---

## 10. Config Change History

### List Changes
- **Endpoint**: `GET /api/v1/config/changes`
- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Response Body**:
  ```json
  [
    {
      "id": 1,
      "actor_type": "api_key",
      "actor_id": "key_6168de90...",
      "resource_type": "routing_rule",
      "resource_id": "route_default_us",
      "action": "create",
      "created_at": "2026-07-05T14:11:23Z"
    }
  ]
  ```

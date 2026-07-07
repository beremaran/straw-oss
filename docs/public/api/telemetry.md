# Telemetry & Observability Read APIs

Straw's Control plane buffers telemetry events and asynchronously writes them to a ClickHouse database. The telemetry read APIs allow tenants to query historical logs, trace outbound request attempts, monitor worker health history, and audit configuration mutations.

---

## Shared Query Parameters & Pagination

All telemetry listing endpoints support cursor-based pagination and window filtering.

### Query Parameters
- **`from`** (string): RFC3339 datetime window start. Default is `24h` ago for requests/workers, and `7d` ago for audit events.
- **`to`** (string): RFC3339 datetime window end. Default is current time.
- **`limit`** (integer): Number of records to return. Default is `100`, max is `500`.
- **`sort`** (string): Sort order. Allowed values: `timestamp_desc`, `timestamp_asc` (default `timestamp_desc`).
- **`cursor`** (string): Cursor token returned in the previous response's `next_cursor` field.

### Envelope Format
List responses return a standard JSON envelope:

```json
{
  "items": [],
  "next_cursor": "eyJ0ZW5hbnRfaWQiOiIyMjIyMjIyMi...",
  "query": {
    "from": "2026-07-07T14:11:23Z",
    "to": "2026-07-08T14:11:23Z",
    "limit": 100,
    "sort": "timestamp_desc"
  }
}
```

---

## 1. Request Telemetry (`GET /api/v1/telemetry/requests`)

List request forwarding telemetry summaries scoped to the caller's tenant.

- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Supported Filter Keys**:
  - `request_id`, `trace_id`, `api_key_ref`, `ingress_type`, `method`, `target_host`, `route_id`, `pool_id`, `executor_type`, `country`, `region`, `ip_type`, `tag`, `upstream_status`, `client_status`, `error_code`, `error_category`, `timeout_type`, `capture_decision`.

### Example Response:
```json
{
  "items": [
    {
      "timestamp": "2026-07-07T17:04:02.123456Z",
      "request_id": "req_1783260685717525503",
      "trace_id": "trace_8168de90dc294fbc",
      "api_key_ref": "key_6168de90...",
      "ingress_type": "rest",
      "method": "GET",
      "target_host": "api.github.com",
      "target_url": "https://api.github.com/users/octocat",
      "route_id": "route_default_us",
      "pool_id": "pool_us_west",
      "executor_type": "egress",
      "country": "US",
      "region": "us-west-1",
      "ip_type": "datacenter",
      "tags": ["high-bandwidth"],
      "attempt_count": 1,
      "upstream_status": 200,
      "client_status": 200,
      "error_code": "",
      "error_category": "",
      "timeout_type": "",
      "request_size_bytes": 1024,
      "response_size_bytes": 4096,
      "timing": {
        "routing_ms": 1,
        "assignment_ms": 4,
        "egress_ms": 120,
        "total_ms": 125
      },
      "capture_decision": "none"
    }
  ],
  "next_cursor": ""
}
```

---

## 2. Request Details (`GET /api/v1/telemetry/requests/{request_id}`)

Fetch full execution detail of a single request ID, including all retry attempts.

- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Behavior**: Returns `404 Not Found` with `route_no_match` if the request ID belongs to a different tenant (preventing enumeration attacks).

### Example Response:
```json
{
  "timestamp": "2026-07-07T17:04:02.123456Z",
  "request_id": "req_1783260685717525503",
  "trace_id": "trace_8168de90dc294fbc",
  "api_key_ref": "key_6168de90...",
  "ingress_type": "rest",
  "method": "GET",
  "target_host": "api.github.com",
  "target_url": "https://api.github.com/users/octocat",
  "route_id": "route_default_us",
  "pool_id": "pool_us_west",
  "executor_type": "egress",
  "country": "US",
  "region": "us-west-1",
  "ip_type": "datacenter",
  "tags": ["high-bandwidth"],
  "attempt_count": 2,
  "upstream_status": 200,
  "client_status": 200,
  "error_code": "",
  "error_category": "",
  "timeout_type": "",
  "request_size_bytes": 1024,
  "response_size_bytes": 4096,
  "timing": {
    "routing_ms": 1,
    "assignment_ms": 5,
    "egress_ms": 150,
    "total_ms": 156
  },
  "capture_decision": "none",
  "attempts": [
    {
      "attempt": 1,
      "timestamp": "2026-07-07T17:04:02.123456Z",
      "client_status": 502,
      "upstream_status": 0,
      "error_code": "upstream_connect_timeout",
      "error_category": "egress",
      "timeout_type": "connection",
      "timing": {
        "routing_ms": 1,
        "assignment_ms": 4,
        "egress_ms": 1000,
        "total_ms": 1005
      }
    },
    {
      "attempt": 2,
      "timestamp": "2026-07-07T17:04:03.200000Z",
      "client_status": 200,
      "upstream_status": 200,
      "error_code": "",
      "error_category": "",
      "timeout_type": "",
      "timing": {
        "routing_ms": 0,
        "assignment_ms": 3,
        "egress_ms": 120,
        "total_ms": 123
      }
    }
  ]
}
```

---

## 3. Worker Event Telemetry (`GET /api/v1/telemetry/workers`)

Query worker lifecycle events (registrations, disconnections, heartbeats).

- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Supported Filter Keys**:
  - `worker_ref`, `executor_type`, `event_type`, `health`, `draining`.

### Example Response:
```json
{
  "items": [
    {
      "timestamp": "2026-07-07T17:00:00Z",
      "worker_ref": "egress-us-west-01",
      "executor_type": "egress",
      "event_type": "heartbeat",
      "health": "healthy",
      "active_requests": 2,
      "max_concurrency": 16,
      "available_capacity": 14,
      "draining": false,
      "reason": ""
    }
  ],
  "next_cursor": ""
}
```

---

## 4. Configuration Audit Telemetry (`GET /api/v1/telemetry/audit`)

Query configuration mutation history (creations, updates, deletions) performed on the tenant.

- **Role**: `tenant_admin`, `operator`, or `viewer`
- **Supported Filter Keys**:
  - `actor_type`, `actor_ref`, `config_type`, `resource_id`, `action`, `field_path`, `config_version`.

### Example Response:
```json
{
  "items": [
    {
      "timestamp": "2026-07-07T17:04:02.123Z",
      "actor_type": "api_key",
      "actor_ref": "key_6168de90...",
      "config_type": "routing_rule",
      "resource_id": "route_default_us",
      "action": "update",
      "config_version": 4,
      "field_path": "target_pool_id",
      "old_value_json": "\"pool_us_east\"",
      "new_value_json": "\"pool_us_west\""
    }
  ],
  "next_cursor": ""
}
```

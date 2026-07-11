# Appendix B - Telemetry Read APIs

This appendix defines the P1 tenant-facing telemetry read contract consumed by:

- `../implementation-history.md#p1-12`
- `../implementation-history.md#p1-13`
- `../implementation-history.md#p1-14`

The APIs expose ClickHouse metadata from Section 22 without exposing internal worker/session topology. Production
handlers must apply these rules at the API layer, even when the underlying ClickHouse rows contain more fields.

## Common Rules

All telemetry endpoints live under `/api/v1/telemetry` and require a key whose tenant and role are authorized to read
tenant telemetry. Platform-scoped keys may query a tenant only when the request explicitly supplies the tenant context
used by the rest of the Control API.

Every query is tenant-scoped. Callers cannot override `tenant_id` with a query parameter, and every ClickHouse query
must include the authenticated tenant predicate before applying optional filters.

Responses use UTC RFC3339 timestamps with millisecond precision. Durations are integer milliseconds. Byte counters are
integers. Empty result sets return `200` with an empty `items` array, not `404`.

The public schemas never expose:

- `worker_id`,
- `session_id`,
- `selected_executor`,
- NATS subjects,
- unsanitized URLs,
- request or response header values,
- credentials or secret config values.

Where a worker identifier is useful, APIs return `worker_ref`: a stable opaque alias scoped to the tenant and derived at
the API layer from `worker_id`. `worker_ref` must not be equal to `worker_id`, must not include the worker credential
ID, and must not encode placement, hostnames, or session IDs.

## Pagination, Sorting, And Limits

List endpoints accept:

| Parameter | Rule |
|-----------|------|
| `from` | Optional inclusive UTC timestamp. Defaults to `now - 1h`. |
| `to` | Optional exclusive UTC timestamp. Defaults to `now`. |
| `limit` | Optional integer `1..500`. Defaults to `100`. |
| `cursor` | Optional opaque cursor returned by the previous page. |
| `sort` | Optional. `timestamp_desc` default; `timestamp_asc` allowed. |

Maximum query windows:

| Endpoint | Maximum window |
|----------|----------------|
| `GET /api/v1/telemetry/requests` | 24 hours |
| `GET /api/v1/telemetry/workers` | 24 hours |
| `GET /api/v1/telemetry/audit` | 7 days |

`GET /api/v1/telemetry/requests/{request_id}` ignores `from`, `to`, `limit`, and `cursor`; it fetches only rows matching
the authenticated tenant and exact `request_id`.

Cursors are opaque and must bind the tenant, endpoint, filter set, sort order, last timestamp, and last stable tie-break
value. A cursor for one endpoint or filter set is invalid for another.

ClickHouse execution limits:

- max execution time: 5 seconds;
- max rows read: 250000;
- max bytes read: 64 MiB;
- max returned rows: endpoint `limit`;
- timeout response: public `timeout_exceeded` error with HTTP `504`;
- ClickHouse read-limit response: public `invalid_request` error telling the caller to narrow the time window or
  filters.

Handlers must reject windows over the endpoint maximum with `invalid_request` before querying ClickHouse.

## Request Telemetry

### `GET /api/v1/telemetry/requests`

Returns request summaries from `request_events`. Multiple attempt rows with the same `request_id` are collapsed into one
item using the latest timestamp as the summary row and `attempt_count` as the count of rows for that request.

Filters:

| Parameter | Source column / rule |
|-----------|----------------------|
| `request_id` | Exact match. |
| `trace_id` | Exact match. |
| `api_key_ref` | Stable public alias for `api_key_id`; never raw `api_key_id`. |
| `ingress_type` | Exact match. |
| `method` | Exact match. |
| `target_host` | Exact match. |
| `route_id` | Exact match; tenant-visible route IDs only. |
| `pool_id` | Exact match; tenant-visible pool IDs only. |
| `executor_type` | Exact match. |
| `country` | Exact match. |
| `region` | Exact match. |
| `ip_type` | Exact match. |
| `tag` | Matches one tag in `tags`. |
| `upstream_status` | Exact integer match. |
| `client_status` | Exact integer match. |
| `error_code` | Exact match. |
| `error_category` | Exact match. |
| `timeout_type` | Exact match. |
| `capture_decision` | Exact match. |

Response item schema:

```json
{
  "timestamp": "2026-07-06T12:00:00.123Z",
  "request_id": "req_...",
  "trace_id": "trc_...",
  "api_key_ref": "key_...",
  "ingress_type": "rest",
  "method": "GET",
  "target_host": "example.com",
  "target_url": "https://example.com/path",
  "route_id": "route_a",
  "pool_id": "pool_a",
  "executor_type": "http",
  "country": "US",
  "region": "us-west-1",
  "ip_type": "datacenter",
  "tags": ["datacenter"],
  "attempt_count": 1,
  "upstream_status": 200,
  "client_status": 200,
  "error_code": "",
  "error_category": "",
  "timeout_type": "",
  "request_size_bytes": 0,
  "response_size_bytes": 1024,
  "timing": {
    "routing_ms": 3,
    "assignment_ms": 4,
    "egress_ms": 123,
    "total_ms": 140
  },
  "capture_decision": "none"
}
```

`target_url` must be the sanitized ClickHouse value from Section 21: no userinfo, no header material, and no query string
unless tenant policy explicitly allows query storage.

### `GET /api/v1/telemetry/requests/{request_id}`

Returns the same public fields as the list endpoint plus an `attempts` array sorted by `attempt` ascending and then
`timestamp` ascending.

```json
{
  "request_id": "req_...",
  "attempts": [
    {
      "attempt": 1,
      "timestamp": "2026-07-06T12:00:00.123Z",
      "client_status": 200,
      "upstream_status": 200,
      "error_code": "",
      "error_category": "",
      "timeout_type": "",
      "timing": {
        "routing_ms": 3,
        "assignment_ms": 4,
        "egress_ms": 123,
        "total_ms": 140
      }
    }
  ]
}
```

Missing records return `404` only after applying the authenticated tenant predicate.

## Worker Telemetry

### `GET /api/v1/telemetry/workers`

Returns public worker health events from `worker_events`.

Filters:

| Parameter | Source column / rule |
|-----------|----------------------|
| `worker_ref` | Stable public alias for `worker_id`. |
| `executor_type` | Exact match. |
| `event_type` | Exact match. |
| `health` | Exact match. |
| `draining` | Boolean. |

Response item schema:

```json
{
  "timestamp": "2026-07-06T12:00:00.123Z",
  "worker_ref": "wrkpub_...",
  "executor_type": "http",
  "event_type": "heartbeat",
  "health": "healthy",
  "active_requests": 2,
  "max_concurrency": 100,
  "available_capacity": 98,
  "draining": false,
  "reason": ""
}
```

`session_id` is omitted. If a worker restarts and receives a new internal session, the public `worker_ref` stays stable
for that tenant worker identity.

## Audit Telemetry

### `GET /api/v1/telemetry/audit`

Returns config audit events from `config_audit_events`. Values are already redacted by Section 21 rules before
ClickHouse insertion; the API must not attempt to reconstruct redacted fields.

Filters:

| Parameter | Source column / rule |
|-----------|----------------------|
| `actor_type` | Exact match. |
| `actor_ref` | Stable public alias for `actor_id`; never raw API key secret material. |
| `config_type` | Exact match. |
| `resource_id` | Exact match. |
| `action` | Exact match. |
| `field_path` | Exact match. |
| `config_version` | Exact integer match. |

Response item schema:

```json
{
  "timestamp": "2026-07-06T12:00:00.123Z",
  "actor_type": "api_key",
  "actor_ref": "key_...",
  "config_type": "routing_rule",
  "resource_id": "route_a",
  "action": "update",
  "config_version": 42,
  "field_path": "enabled",
  "old_value_json": "false",
  "new_value_json": "true"
}
```

Secret values appear only as `[redacted]`; sensitive values appear only as hashes or bounded metadata, matching the
stored audit event.

## Response Envelope

List endpoints return:

```json
{
  "items": [],
  "next_cursor": "",
  "query": {
    "from": "2026-07-06T11:00:00.000Z",
    "to": "2026-07-06T12:00:00.000Z",
    "limit": 100,
    "sort": "timestamp_desc"
  }
}
```

`next_cursor` is empty when no more results are available.

## Implementation Test Rows

Before P1 telemetry implementation is marked complete, task 12 must cover at least these rows:

| Area | Required checks | Owning task |
|------|-----------------|-------------|
| Tenant isolation | Requests, worker events, and audit events from another tenant are never returned by list, detail, or cursor continuation queries. | `../implementation-history.md#p1-12` |
| Topology redaction | Public responses omit raw `worker_id`, `session_id`, and `selected_executor`; worker filters use only `worker_ref`. | `../implementation-history.md#p1-12` |
| URL and secret redaction | Request telemetry returns sanitized `target_url`; audit telemetry preserves `[redacted]` secret values and does not expose credential secrets. | `../implementation-history.md#p1-12` |
| Query bounds | Over-wide windows, bad timestamps, bad limits, unsupported sorts, and mismatched cursors fail before broad ClickHouse scans. | `../implementation-history.md#p1-12` |
| Pagination | Limit, sort direction, tie-break ordering, and cursor continuation are stable and tenant-bound. | `../implementation-history.md#p1-12` |
| Detail lookup | Request detail applies tenant scope before returning attempts or `404`. | `../implementation-history.md#p1-12` |
| ClickHouse limits | Query timeout and read-limit failures return public errors without leaking ClickHouse internals. | `../implementation-history.md#p1-12` |

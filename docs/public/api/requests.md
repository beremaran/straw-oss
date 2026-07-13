# Request API

`POST /api/v1/requests` performs one upstream HTTP or HTTPS request.

```sh
curl -sS http://localhost:8080/api/v1/requests \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer your-deployment-token' \
  -d '{
    "method": "POST",
    "url": "https://httpbin.org/anything",
    "headers": [{"name":"Content-Type","value_base64":"YXBwbGljYXRpb24vanNvbg=="}],
    "body": {"mode":"inline_base64","data_base64":"eyJoZWxsbyI6IndvcmxkIn0="},
    "routing": {
      "tags": ["datacenter"],
      "country": "AU",
      "region": "ap-southeast-2",
      "ip_type": "residential",
      "sticky_session_id": "checkout-session-42"
    },
    "timeout_ms": 30000,
    "replayable": false
  }'
```

## Request fields

| Field | Required | Description |
| --- | --- | --- |
| `method` | yes | HTTP method. |
| `url` | yes | Absolute `http` or `https` URL. URL user info is rejected. |
| `headers` | no | Ordered `{name,value_base64}` entries. Duplicate names are preserved. |
| `body` | no | Inline `{mode:"inline_base64",data_base64:"..."}` or verified `{mode:"receipt",receipt_id:"..."}`. |
| `routing` | no | Worker-routing constraints. Omit it when the deployment's ordinary route should choose the worker. |
| `routing.tags` | no | Non-empty unique string tags required of the worker. Up to 32 tags; each is at most 64 bytes. |
| `routing.country` | no | Two-letter ISO country code constraint; Straw normalizes it to uppercase. |
| `routing.region` | no | Non-empty region identifier constraint, up to 128 bytes. |
| `routing.ip_type` | no | Non-empty worker IP-type constraint, up to 128 bytes. |
| `routing.sticky_session_id` | no | Session identifier used to prefer the same eligible worker while its rule pin remains valid. |
| `response_body_mode` | no | `inline_base64` (default) or `receipt` when the object-storage profile is enabled. |
| `fingerprint_profile` | no | Exact, case-sensitive built-in profile name. Omit it for ordinary TLS. See the [fingerprint catalogue](../compatibility.md#fingerprint-profile-catalogue). |
| `timeout_ms` | no | Total deadline, from 1000 ms through the configured maximum. |
| `replayable` | no | Permits safe transport retry. Clients default GET, HEAD, and OPTIONS to true. |

Hop-by-hop headers, `Host`, `Content-Length`, and proxy authorization headers are managed or rejected by Straw.
Request bodies default to a 1 MiB limit.
That limit applies to inline bodies; receipt bodies use `object_storage.max_object_bytes` and must pass the receipt
size/checksum flow before assignment. See [Object storage and receipts](../object-storage-receipts.md).

Fingerprinting controls TLS ClientHello and, when negotiated, HTTP/2 settings, flow-control, pseudo-header ordering,
and priority behavior. It does not synthesize browser application headers, cookies, JavaScript, or browser state.
HTTP/3 is not supported. A named request fails with `unsupported_fingerprint` if the selected worker does not
advertise the exact profile.

## Routing behavior

Routing rules are evaluated in ascending priority order. A configured rule constraint must be present in the request
and match; an omitted routing value does not silently satisfy a configured `country`, `region`, `ip_type`, ingress,
or host constraint. When a routing hint is supplied, the selected worker must advertise the corresponding capability;
a worker with a missing capability claim is not treated as a wildcard. Tags require the worker to advertise every
requested tag.

`sticky_session_id` pins selection to the worker previously selected for that deployment and session while the rule's
sticky TTL is valid. If that worker is unavailable, the rule's `allow_sticky_fallback` setting determines whether Straw
may select another eligible worker or returns `sticky_session_unavailable`. If every otherwise eligible worker is at
capacity, Straw returns `executor_capacity_exhausted`. With no routing hints, normal rule priority and worker capacity
selection apply.

The same `RoutingHints` contract is available to the forward-proxy ingress through the authenticated
`X-Straw-Route-Tags`, `X-Straw-Route-Country`, `X-Straw-Route-Region`, `X-Straw-Route-IP-Type`, and
`X-Straw-Route-Sticky-Session` headers. See [HTTP and HTTPS proxy ingress](../proxy-ingress.md#routing-hints) for
the bounded syntax and header-stripping rules.

## Success

Control returns HTTP `200` when Straw transported the request, even if the destination returned an error status.

```json
{
  "request_id": "req_...",
  "status": 200,
  "headers": [{"name":"Content-Type","value_base64":"dGV4dC9odG1s"}],
  "body": {"mode":"inline_base64","data_base64":"...","truncated":false},
  "timing": {"routing_ms":0,"assignment_ms":1,"egress_ms":82,"total_ms":84}
}
```

The `timing` phases map onto the stages a request passes through:

```mermaid
flowchart LR
  A["Validate and<br/>select worker"] --> B["Worker acknowledges<br/>assignment"] --> C["Outbound request<br/>and response"] --> D["Response<br/>returned"]
  A -.- t1["routing_ms"]
  B -.- t2["assignment_ms"]
  C -.- t3["egress_ms"]
  D -.- t4["total_ms"]
```

`body.truncated` is reserved for compatibility and is currently false. If the upstream body exceeds
`max_inline_response_body_bytes`, Straw returns `body_too_large` instead of a partial success.

With `response_body_mode:"receipt"`, `body` instead contains `mode`, `receipt_id`, `size_bytes`, and `sha256_hex`.
The authorized content endpoint and explicit expiry are returned by `GET /api/v1/receipts/{receipt_id}`.

## Errors

Straw failures use an outer 4xx or 5xx status and a stable envelope:

```json
{
  "category": "client",
  "code": "invalid_request",
  "message": "request URL must use http or https",
  "retryable": false,
  "request_id": "req_..."
}
```

Optional fields are `timeout_type`, `retry_after_ms`, and `details`. Use `code` for program logic and `message` for
humans. Retry only when `retryable` is true and the original operation is safe to replay.

| Stable code | Category | HTTP / retryable | Meaning |
| --- | --- | --- | --- |
| `auth_failure` | client | 401 / no | invalid deployment token |
| `invalid_request`, `header_injection_failed`, `unsupported_ingress_mode` | client | 400 / no | malformed input or unsupported request behavior |
| `destination_denied` | client | 403 / no | destination policy denied the target |
| `route_no_match` | routing | 404 / no | no routing rule matched |
| `route_unavailable`, `executor_capacity_exhausted` | routing | 503 / yes | no eligible capacity currently exists |
| `sticky_session_unavailable` | routing | 503 / no | required sticky worker is unavailable |
| `assignment_timeout`, `transport_unavailable` | transport | 504 / yes | assignment/NATS did not complete |
| `worker_disconnected` | transport | 502 / yes | worker disappeared mid-request |
| `protocol_error`, `unsupported_fingerprint` | transport | 502 or 400 / no | invalid protocol sequence or unsupported requested profile |
| `timeout_exceeded` | transport | 504 / no | total deadline expired; `timeout_type` identifies the stage |
| `upstream_dns_failure`, `upstream_tls_failure`, `upstream_connection_refused`, `upstream_reset`, `upstream_proxy_failure` | egress | 502 / yes | upstream resolution, TLS, connection, reset, or proxy failure |
| `upstream_connect_timeout` | egress | 504 / yes | upstream connection deadline expired |
| `stream_upload_aborted`, `stream_download_aborted` | streaming | 502 / no | bounded stream was interrupted |
| `body_ref_unavailable` | streaming | 409 / no | receipt is missing, expired, or ineligible |
| `body_too_large` | streaming | 413 / no | configured inline/request/response limit exceeded |
| `control_internal_error` | control | 500 / no | unexpected Control failure |
| `executor_internal_error` | egress | 502 / no | unexpected Egress failure |
| `cancelled` | client | 499 / no | caller or operator cancelled the request |

The table is generated from the semantic registry contract: changing any code requires compatibility notes and the
public-surface drift check verifies every registry string remains represented here.

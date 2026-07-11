---
sidebar_position: 1
---

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
| `body` | no | `{mode:"inline_base64",data_base64:"..."}`. |
| `fingerprint_profile` | no | Supported profile name; the built-in deployment enables `chrome_120`. |
| `timeout_ms` | no | Total deadline, from 1000 ms through the configured maximum. |
| `replayable` | no | Permits safe transport retry. Clients default GET, HEAD, and OPTIONS to true. |

Hop-by-hop headers, `Host`, `Content-Length`, and proxy authorization headers are managed or rejected by Straw.
Request bodies default to a 1 MiB limit.

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

`body.truncated` is reserved for compatibility and is currently false. If the upstream body exceeds
`max_inline_response_body_bytes`, Straw returns `body_too_large` instead of a partial success.

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

Common codes include `auth_failure`, `invalid_request`, `destination_denied`, `route_unavailable`,
`assignment_timeout`, `connect_timeout`, `response_header_timeout`, `upstream_reset`, `body_too_large`, and
`control_internal_error`.

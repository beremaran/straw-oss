## 7. Public API Surface

The canonical public base path is `/api/v1`.

P0 exposes:

- `POST /api/v1/requests` for synchronous REST request transport,
- `/api/v1/config/*` for durable configuration resources,
- `/api/v1/admin/*` for runtime operational actions,
- `/metrics` for Prometheus metrics.

P1 may add telemetry read APIs and proxy ingress endpoints. P2 may add MITM CA distribution and BodyRef-related APIs.

### REST Request Transport — P0

P0 implements externally synchronous REST transport:

```http
POST /api/v1/requests
Authorization: Bearer <api_key>
Content-Type: application/json
```

The external P0 REST contract is non-streaming:

1. The client sends a JSON request envelope.
2. The request body, if present, uses `inline_base64` and must not exceed `request.max_inline_request_body_bytes`.
3. Control may internally split the body into NATS `DataFrame`s.
4. Egress may internally stream response frames back to Control.
5. Control buffers the upstream response until complete or until `request.max_inline_response_body_bytes` is exceeded.
6. Control returns either a JSON success envelope or a public ErrorResponse.

P0 does not expose HTTP chunked response streaming, BodyRef downloads, CONNECT, MITM, or raw upstream response
passthrough.

#### Request Schema

```json
{
  "method": "GET",
  "url": "https://example.com/path?x=1",
  "headers": [
    {
      "name": "User-Agent",
      "value_base64": "..."
    }
  ],
  "body": {
    "mode": "inline_base64",
    "data_base64": ""
  },
  "routing": {
    "tags": [
      "datacenter"
    ],
    "country": "US",
    "region": "us-west-1",
    "ip_type": "datacenter",
    "sticky_session_id": "optional-session"
  },
  "fingerprint_profile": "chrome_120",
  "timeout_ms": 60000,
  "replayable": false,
  "capture_hint": "none"
}
```

Field rules:

- `method` is required.
- `method` must not be `CONNECT` in P0 REST transport.
- `url` is required and must be absolute HTTP or HTTPS.
- URL userinfo is rejected.
- `headers` preserves order and duplicates.
- Header values are bytes and use Base64 in JSON.
- `body.mode` must be `inline_base64` or omitted in P0.
- `routing` fields are hints and hard constraints when supplied.
- `fingerprint_profile` is optional; tenant default applies when absent.
- `timeout_ms` is capped by tenant and deployment limits.
- `replayable` defaults to `false` except prototype/generated clients may default `GET`, `HEAD`, and `OPTIONS` to
  `true` before submission.
- `capture_hint` must be absent or `none` in P0. Any other value returns `invalid_request`.
- Redirect following is not available in P0. Redirect responses pass through as upstream responses.

#### Successful Response Schema

For REST transport, successful upstream responses are represented in a JSON envelope because the REST request itself is
JSON. HTTP proxy/MITM decoded modes later stream raw upstream responses directly.

```json
{
  "request_id": "req_...",
  "status": 200,
  "headers": [
    {
      "name": "Content-Type",
      "value_base64": "..."
    }
  ],
  "body": {
    "mode": "inline_base64",
    "data_base64": "...",
    "truncated": false
  },
  "timing": {
    "routing_ms": 3,
    "assignment_ms": 4,
    "egress_ms": 123,
    "total_ms": 140
  }
}
```

If Straw successfully transports the request and receives upstream status/headers, `POST /api/v1/requests` returns HTTP
`200` from the Straw API even when the upstream status is `301`, `404`, `429`, or `500`. The upstream status is carried
in the JSON envelope as `status`.

If Straw fails to authenticate, authorize, validate, route, assign, stream, execute, or complete the transport, Control
returns the public ErrorResponse with the HTTP status from the canonical error registry.

If the upstream response body exceeds `request.max_inline_response_body_bytes` before P2 large-body transport or a P1
streaming endpoint exists, Control returns `body_too_large` with `details.direction = "response"`.

### REST Streaming Variant — P1

P1 may add:

```http
POST /api/v1/requests:stream
```

This endpoint streams response bytes and metadata using HTTP chunking or server-selected framing. The exact framing must
be specified before implementation.

### Config and Admin APIs

Durable configuration endpoints live under:

```text
/api/v1/config/*
```

Runtime operational actions live under:

```text
/api/v1/admin/*
```

Config and admin endpoints require `operator`, `tenant_admin`, or `system_admin` according to the endpoint-specific
table in Section 26.

### Telemetry Read APIs — P1

The role model reserves telemetry read permissions, but P0 does not implement a general telemetry query API. P0 may
return the current request's metadata in the request response envelope and may expose local health/metrics endpoints.

P1 adds explicit APIs such as:

```text
GET /api/v1/telemetry/requests
GET /api/v1/telemetry/requests/{request_id}
GET /api/v1/telemetry/workers
GET /api/v1/telemetry/audit
```

These are placeholders until P1 schemas and ClickHouse query limits are specified.

### MITM CA Distribution Endpoint — P2

P2 adds:

```http
GET /api/v1/mitm/ca.pem
Authorization: Bearer <api_key>
```

Any authenticated key whose tenant is allowed to use MITM may download the public CA certificate. Tenant admin rights
are required to rotate or configure the CA, but not to download the public certificate needed by clients.

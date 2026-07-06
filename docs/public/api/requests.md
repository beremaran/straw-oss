# REST Request Forwarding API

Egress clients forward outbound requests by submitting a JSON envelope to the REST transport endpoint. The Control plane routes the request to an appropriate Egress worker, which executes it against the upstream host and returns the result.

---

## Endpoint Details

- **Method**: `POST`
- **Path**: `/api/v1/requests`
- **Authentication**: Required (`Authorization: Bearer <api_key>`)
- **Required Role**: `requester` or `tenant_admin` (Platform-scoped keys are rejected)
- **Headers**:
  - `Content-Type: application/json`

---

## Request Payload Schema

```json
{
  "method": "GET",
  "url": "https://example.com/path?query=val",
  "headers": [
    {
      "name": "User-Agent",
      "value_base64": "bXktY3VzdG9tLXVzZXItYWdlbnQ="
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
    "sticky_session_id": "optional-session-id"
  },
  "fingerprint_profile": "chrome_120",
  "timeout_ms": 15000,
  "replayable": false,
  "capture_hint": "none"
}
```

### Request Field Descriptions

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| **method** | `string` | Yes | HTTP method to perform (e.g. `GET`, `POST`, `PUT`, `DELETE`). Must be uppercase. `CONNECT` is rejected. |
| **url** | `string` | Yes | The absolute URL of the upstream destination. Must start with `http://` or `https://`. Fragments (`#...`) and Userinfo (`user:pass@host`) are rejected. |
| **headers** | `array` | No | List of header objects to forward. Header names must be valid HTTP tokens. Value must be a base64-encoded string representing the bytes. |
| **body** | `object` | No | Body wrapper. If provided, `body.mode` must be `inline_base64`, and `body.data_base64` must contain the base64-encoded payload. |
| **routing** | `object` | No | Constraint hints for worker matching. |
| **routing.tags** | `array` | No | String tags required of the Egress worker executing the request. |
| **routing.country** | `string` | No | ISO country code constraint (e.g. `US`, `GB`). |
| **routing.region** | `string` | No | Region constraint (e.g. `us-west-1`). |
| **routing.ip_type** | `string` | No | Required worker IP type (`datacenter`, `residential`, etc.). |
| **routing.sticky_session_id** | `string` | No | Session identifier. Requests with the same identifier will prefer the same worker instance while it remains available. |
| **fingerprint_profile** | `string` | No | Handshake fingerprint preset name (e.g. `chrome_120`). If omitted, the tenant's default is used. |
| **timeout_ms** | `integer` | No | Request execution timeout in milliseconds. Minimum is `1000`. Cannot exceed the tenant or control plane configuration limits. |
| **replayable** | `boolean` | No | Defaults to `false`. Indicates if the request is safe to retry by the client/control plane upon connection drops. |
| **capture_hint** | `string` | No | Omit this field or set it to `none`. Other capture modes are rejected. |

---

## Validation & Normalization Rules

Straw applies strict validation checks to all incoming request envelopes before scheduling execution:

1. **Target URL**:
   - URL fragments (e.g. `https://host/path#section`) and HTTP basic auth credentials (e.g. `https://user:pass@host/`) are strictly rejected.
   - Destination-policy checks lowercase ASCII hostnames and trim a trailing dot. Non-ASCII hostnames are rejected.
   - Default ports are inferred for outbound execution when the URL omits a port (`80` for HTTP, `443` for HTTPS).
2. **HTTP Headers**:
   - Header count is capped at `64` total headers.
   - Header names cannot exceed `64` bytes.
   - Total aggregated header size cannot exceed `16384` bytes.
   - Header names and values must not contain bare Carriage Return (`\r` / `0x0D`) or Line Feed (`\n` / `0x0A`) characters (preventing header injection).
   - Client-supplied `Host` is rejected during request validation. `Content-Length`, `Transfer-Encoding`, `Connection`, `Proxy-Authorization`, and `X-Straw-*` headers are rejected before outbound execution.
3. **HTTP Method**:
   - Must be a valid HTTP token and uppercase. `CONNECT` is not supported in the REST request transport.
4. **Body Size**:
   - If present, the decoded body length must not exceed the control plane's configured limit (`max_inline_request_body_bytes`, default `1048576` bytes / 1 MiB).
5. **Request Timeout**:
   - Must be at least `1000` ms and cannot exceed `max_timeout_ms` (default `120000` ms / 120 seconds).
6. **Strict Mode**:
   - Any unknown fields in the JSON envelope will result in an immediate `invalid_request` validation failure.

---

## Successful Response Schema

When Straw successfully contacts the upstream server, it returns an HTTP `200 OK` wrapper envelope containing details of the upstream response:

```json
{
  "request_id": "req_1783260685717525503",
  "status": 200,
  "headers": [
    {
      "name": "Content-Type",
      "value_base64": "dGV4dC9odG1s"
    }
  ],
  "body": {
    "mode": "inline_base64",
    "data_base64": "PGh0bWw+...",
    "truncated": false
  },
  "timing": {
    "routing_ms": 0,
    "assignment_ms": 3,
    "egress_ms": 290,
    "total_ms": 301
  }
}
```

### Response Field Descriptions

- **request_id**: Unique request trace ID generated by the control plane.
- **status**: The HTTP status code returned by the remote upstream server (e.g. `200`, `302`, `404`, `500`).
- **headers**: Array of HTTP headers returned by the upstream server, with values encoded in base64.
- **body.mode**: Always `inline_base64` in the current REST transport.
- **body.data_base64**: Base64-encoded raw response body.
- **body.truncated**: Set to `true` if the upstream response body exceeded the configured `max_inline_response_body_bytes` limit and was clipped.
- **timing**: Execution phase duration statistics in milliseconds.

---

## Error Response Envelope

If Straw fails to process, route, schedule, or execute the request, it returns a standard JSON error envelope with an appropriate HTTP status code:

```json
{
  "category": "client",
  "code": "invalid_request",
  "message": "Malformed request or missing business fields",
  "retryable": false,
  "request_id": "req_1783260685717525503",
  "details": {
    "reason": "unknown field 'foo'"
  }
}
```

### Canonical Error Codes

Below is the list of error codes that can be returned in the `code` field:

| Code | HTTP Status | Category | Retryable | Description |
| :--- | :--- | :--- | :--- | :--- |
| `auth_failure` | 401 | `client` | No | API key is missing, invalid, or revoked. |
| `tenant_not_found` | 401 | `client` | No | Key references a missing, suspended, or deleted tenant. |
| `insufficient_permissions` | 403 | `client` | No | RBAC validation failed for the requested endpoint. |
| `rate_limit_exceeded` | 429 | `client` | Yes | The tenant's request rate has exceeded their configured limit. |
| `quota_exhausted` | 429 | `client` | Yes | The tenant's monthly bandwidth or request quota is exhausted. |
| `invalid_request` | 400 | `client` | No | Request validation failed (e.g., malformed JSON, invalid timeout, injection characters in headers). |
| `destination_denied` | 403 | `client` | No | Destination matched a configured Tenant Deny Rule. |
| `header_injection_failed` | 400 | `client` | No | A dynamic header injection rule failed evaluation. |
| `conflict` | 409 | `client` | No | Config version conflict. An expected config version mismatch occurred. |
| `unsupported_ingress_mode` | 400 | `client` | No | Request method is not supported (e.g., calling non-POST on requests API). |
| `route_no_match` | 404 | `routing` | No | No routing rule matches the requested destination host and tags. |
| `route_unavailable` | 503 | `routing` | Yes | Matching rule was found, but no Egress workers are currently eligible or online. |
| `sticky_session_unavailable`| 503 | `routing` | No | The sticky session target worker is offline and fallback is disabled. |
| `executor_capacity_exhausted`| 503 | `routing` | Yes | All eligible Egress workers for the destination pool are at concurrency capacity. |
| `assignment_timeout` | 504 | `transport` | Yes | Control plane did not receive a worker assignment acknowledgement within the timeout. |
| `worker_disconnected` | 502 | `transport` | Yes | The assigned Egress worker disconnected from NATS while executing the request. |
| `transport_unavailable` | 504 | `transport` | Yes | The internal NATS transport scheduling layer is unreachable. |
| `protocol_error` | 502 | `transport` | No | An invalid frame sequence or packet format was received from the worker. |
| `timeout_exceeded` | 504 | `transport` | No | The execution exceeded the request's timeout limit. |
| `unsupported_fingerprint` | 400 | `transport` | No | The matched Egress worker cannot apply the requested fingerprint preset. |
| `upstream_dns_failure` | 502 | `egress` | Yes | DNS resolution failed for the target hostname at the Egress worker. |
| `upstream_tls_failure` | 502 | `egress` | Yes | TLS handshake or certificate validation failed with the target server. |
| `upstream_connection_refused`| 502 | `egress` | Yes | The remote server actively refused the connection. |
| `upstream_connect_timeout` | 504 | `egress` | Yes | Connection attempt to the remote server timed out. |
| `upstream_reset` | 502 | `egress` | Yes | Remote server closed the TCP connection before sending a full response. |
| `upstream_proxy_failure` | 502 | `egress` | Yes | The Egress worker's upstream proxy failed. |
| `stream_upload_aborted` | 502 | `streaming` | No | Upload stream was interrupted. |
| `stream_download_aborted` | 502 | `streaming` | No | Download stream was interrupted. |
| `body_ref_unavailable` | 502 | `streaming` | Yes | Referenced body object was unavailable. |
| `body_too_large` | 413 | `streaming` | No | Request or response inline body size exceeded the configured limit. |
| `control_internal_error` | 500 | `control` | No | An unexpected internal error occurred on the Control plane. |
| `executor_internal_error` | 502 | `egress` | No | An unexpected error occurred on the Egress worker daemon. |
| `cancelled` | 499 | `client` | No | The request was explicitly cancelled by the client or an administrator. |

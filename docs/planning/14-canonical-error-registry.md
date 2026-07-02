## 14. Canonical Error Registry

Origin HTTP statuses are not Straw errors. If the origin returns 404, 403, 429, or 500 and Straw successfully
transported the request, Straw returns/reports that as an upstream response, not an ErrorResponse.

Straw errors mean the Straw system failed to authenticate, authorize, validate, route, assign, transport, stream,
execute, or complete the request.

### Categories

| Category    | Scope                 | Meaning                                                        |
|-------------|-----------------------|----------------------------------------------------------------|
| `CLIENT`    | Control-local         | Auth, permissions, validation, rate limits, quotas, deny rules |
| `ROUTING`   | Control-local         | Route or eligible executor selection failed                    |
| `TRANSPORT` | Control↔executor      | NATS, assignment, worker loss, protocol failure                |
| `EGRESS`    | Executor/upstream leg | DNS, connect, TLS, upstream transport failure                  |
| `STREAMING` | Mid-stream transfer   | Upload/download/body transport failure                         |
| `CONTROL`   | Control internal      | Unexpected Control failure                                     |

### Error Codes

| Code | Name                          | Category  |    HTTP | Retryable | Notes                                                |
|-----:|-------------------------------|-----------|--------:|----------:|------------------------------------------------------|
|    1 | `auth_failure`                | CLIENT    |     401 |        No | Invalid API key or worker token                      |
|    2 | `tenant_not_found`            | CLIENT    |     401 |        No | Key references missing/deleted tenant                |
|    3 | `insufficient_permissions`    | CLIENT    |     403 |        No | RBAC failure                                         |
|    4 | `rate_limit_exceeded`         | CLIENT    |     429 |     Later | Uses `retry_after_ms`                                |
|    5 | `quota_exhausted`             | CLIENT    |     429 |     Later | Uses `retry_after_ms` when known                     |
|    6 | `invalid_request`             | CLIENT    |     400 |        No | Malformed request or missing business fields         |
|    7 | `destination_denied`          | CLIENT    |     403 |        No | Deny rule matched                                    |
|    8 | `header_injection_failed`     | CLIENT    |     400 |        No | Resolved injection invalid                           |
|    9 | `conflict`                    | CLIENT    |     409 |        No | Config version conflict                              |
|   10 | `unsupported_ingress_mode`    | CLIENT    |     400 |        No | Unsupported mode for endpoint/route                  |
|  100 | `route_no_match`              | ROUTING   | 421/404 |        No | No rule matched; REST uses 404, proxy uses 421       |
|  101 | `route_unavailable`           | ROUTING   |     503 |       Yes | Rule matched but no eligible executor                |
|  102 | `sticky_session_unavailable`  | ROUTING   |     503 |        No | Sticky target unavailable and fallback not allowed   |
|  103 | `executor_capacity_exhausted` | ROUTING   |     503 |       Yes | All eligible executors at capacity                   |
|  200 | `assignment_timeout`          | TRANSPORT |     504 |       Yes | No AssignAck before timeout                          |
|  201 | `worker_disconnected`         | TRANSPORT |     502 |       Yes | Worker lost mid-request                              |
|  202 | `transport_unavailable`       | TRANSPORT |     504 |       Yes | NATS publish/request/reply unavailable               |
|  203 | `protocol_error`              | TRANSPORT |     502 |        No | Invalid frame/order/sequence                         |
|  204 | `timeout_exceeded`            | TRANSPORT |     504 |     Maybe | See TimeoutType                                      |
|  205 | `unsupported_fingerprint`     | TRANSPORT |     400 |        No | Executor cannot apply requested preset               |
|  300 | `upstream_dns_failure`        | EGRESS    |     502 |       Yes | DNS resolution failed                                |
|  301 | `upstream_tls_failure`        | EGRESS    |     502 |       Yes | TLS handshake/cert failure                           |
|  302 | `upstream_connection_refused` | EGRESS    |     502 |       Yes | Connect refused                                      |
|  303 | `upstream_connect_timeout`    | EGRESS    |     504 |       Yes | Could not connect before timeout                     |
|  304 | `upstream_reset`              | EGRESS    |     502 |       Yes | Upstream closed/reset before complete response       |
|  305 | `upstream_proxy_failure`      | EGRESS    |     502 |       Yes | Configured upstream proxy failed                     |
|  400 | `stream_upload_aborted`       | STREAMING |     502 |     Maybe | Client/control upload interrupted                    |
|  401 | `stream_download_aborted`     | STREAMING |     502 |     Maybe | Executor/upstream download interrupted               |
|  402 | `body_ref_unavailable`        | STREAMING |     502 |       Yes | P2 BodyRef object/stream unavailable                 |
|  403 | `body_too_large`              | STREAMING |     413 |        No | Request or response exceeds configured limit         |
|  500 | `control_internal_error`      | CONTROL   |     500 |        No | Unexpected Control failure                           |
|  501 | `executor_internal_error`     | EGRESS    |     502 |     Maybe | Unexpected executor failure                          |
|  502 | `cancelled`                   | CLIENT    | 499/400 |        No | Client/admin cancellation; status depends on ingress |

There is no separate `response_body_too_large` code. Use `body_too_large` with public-safe ErrorResponse details:

```json
{
  "direction": "request | response",
  "limit_bytes": "1048576"
}
```

### TimeoutType

`timeout_exceeded` uses one of:

- `ASSIGNMENT_TIMEOUT`,
- `CONNECT_TIMEOUT`,
- `REQUEST_HEADER_TIMEOUT`,
- `IDLE_TIMEOUT`,
- `UPLOAD_TIMEOUT`,
- `DOWNLOAD_TIMEOUT`,
- `TOTAL_DEADLINE_TIMEOUT`.

Timeouts are request-scoped and capped by tenant/deployment configuration.

# Appendix C - HTTP/2 Semantics

This appendix defines the P2 HTTP/2 protocol semantics consumed by:

- [P2 task 14](../implementation-history.md#p2-14)
- [P2 task 15](../implementation-history.md#p2-15)
- [P2 task 16](../implementation-history.md#p2-16)

HTTP/2 support is optional, feature-flagged, and must fail-safe back to HTTP/1.1 when unavailable or disabled.

---

## 1. Feature Flags and Config

HTTP/2 is disabled by default at both the ingress and egress boundaries. Enabling HTTP/2 requires explicit static configuration keys:

| Config Key | Default | Behavior |
|------------|---------|----------|
| `egress.http2.enabled` | `false` | When `false`, Egress disables outbound HTTP/2 negotiation and keep-alives do not negotiate HTTP/2 ALPN. |
| `control.http2.enabled` | `false` | When `false`, Control omits `"h2"` from ALPN negotiation on public entrypoints, forcing clients to HTTP/1.1. |

---

## 2. One `request_id` per HTTP/2 Stream

HTTP/2 multiplexes multiple streams over a single TCP connection. The Straw control plane and egress workers map each stream 1-to-1 to a unique Straw request.

### Ingress (Client to Control)
1. When a client establishes an HTTP/2 connection to Control (e.g., for REST or proxy ingress), each HTTP/2 stream represents a single logical request.
2. Control assigns a unique, dot-free `request_id` and a `trace_id` to each stream (or extracts them from headers if present and authorized).
3. The multiplexed streams remain isolated at the request layer; authorization, quota, routing, and dispatch occur independently for each `request_id`.

### Egress (Worker to Upstream)
1. Each NATS assignment carries exactly one `request_id` and is processed on a single Egress worker goroutine.
2. When the egress worker connects to the upstream origin via HTTP/2, it initiates a single stream on the connection.
3. If connection pooling is enabled, multiple worker goroutines processing different `request_id`s may share the same TCP/TLS connection, sending their respective requests on concurrent HTTP/2 streams.
4. Egress must maintain a thread-safe map of active `request_id` to HTTP/2 stream reference on the connection.

---

## 3. Stream Cancellation Mapping

Cancellation signals must propagate immediately across NATS subjects and HTTP/2 stream boundaries to prevent resource leaks.

### Control to Egress (Client Disconnect / Admin Cancel / Timeout)
1. If an HTTP/2 client closes or resets a stream (sends `RST_STREAM` with `CANCEL` or `NO_ERROR`), Control intercepts this and publishes a `CancelFrame` over NATS on the request's `c2e` subject.
2. If Control cancels a request due to deadline expiry or an admin cancel API call:
   - Control publishes a NATS `CancelFrame` to Egress.
   - If the client-side HTTP/2 stream is still active, Control sends an `RST_STREAM` frame with error code `CANCEL` (0x8) or `INTERNAL_ERROR` (0x2) depending on whether it was a deliberate client-side/admin cancel or an internal error.

### Egress to Upstream (NATS Cancel received)
1. Upon receiving a `CancelFrame` on the `c2e` subject, Egress locates the active HTTP/2 stream for the `request_id` and immediately transmits an `RST_STREAM` frame with error code `CANCEL` (0x8) to the upstream origin.
2. The local Egress context for the request is cancelled, and the worker goroutine terminates.

### Upstream to Egress (Upstream Reset)
1. If the upstream origin resets the HTTP/2 stream (sends `RST_STREAM`), Egress catches the error code and maps it to a canonical Straw `ErrorCode` before publishing an `ErrorFrame` or `CancelledFrame` on the NATS `e2c` subject:

| HTTP/2 RST_STREAM Error Code | Canonical Straw ErrorCode | Description / Handling |
|-----------------------------|---------------------------|------------------------|
| `NO_ERROR` (0x0)            | None / Success            | Clean stream termination. |
| `PROTOCOL_ERROR` (0x1)      | `upstream_reset`          | Upstream protocol violation. |
| `INTERNAL_ERROR` (0x2)      | `upstream_reset`          | Upstream internal error. |
| `FLOW_CONTROL_ERROR` (0x3)  | `upstream_reset`          | Flow control window violation. |
| `SETTINGS_TIMEOUT` (0x4)    | `upstream_reset`          | Connection setting timeout. |
| `STREAM_CLOSED` (0x5)       | `upstream_reset`          | Stream is already closed. |
| `FRAME_SIZE_ERROR` (0x6)    | `upstream_reset`          | Frame exceeds max payload. |
| `REFUSED_STREAM` (0x7)      | `upstream_reset`          | Upstream refused stream. Egress may retry if before `RequestStart` ack. |
| `CANCEL` (0x8)              | `upstream_reset`          | Upstream cancelled the stream. |
| `COMPRESSION_ERROR` (0x9)   | `upstream_reset`          | HPACK decompression failure. |
| `CONNECT_ERROR` (0xa)       | `upstream_connection_refused` | Inability to establish connection. |
| `ENHANCE_YOUR_CALM` (0xb)   | `upstream_reset`          | Upstream rate-limiting. Add `"enhance_your_calm"` diagnostic details. |
| `INADEQUATE_SECURITY` (0xc) | `upstream_tls_failure`    | Rejected TLS profile or cipher. |
| `HTTP_1_1_REQUIRED` (0xd)   | `upstream_reset`          | Triggers HTTP/1.1 fallback retry (see Section 9). |

---

## 4. Flow-Control Interaction with NATS Credit

Straw relies on per-request byte-credit to prevent memory exhaustion. HTTP/2 stream-level flow control (`WINDOW_UPDATE` frames) must be tied directly to NATS credit.

### Inbound Flow Control (Client to Control upload)
1. Control initializes the client HTTP/2 stream receive window with a size bounded by `control.transport.initial_upload_credit_bytes` (default 8 MiB).
2. Control consumes client-sent HTTP/2 `DATA` frame bytes and forwards them as NATS `DataFrame`s on `c2e` to Egress.
3. Control must **withhold** sending HTTP/2 `WINDOW_UPDATE` frames to the client if the NATS upload credit for the request (granted by Egress via `CreditFrame`) is exhausted.
4. When Egress publishes a NATS `CreditFrame` to Control, Control increments the request's local credit counter and sends an HTTP/2 `WINDOW_UPDATE` frame to the client stream to allow the client to resume uploading.

### Outbound Flow Control (Upstream to Egress download)
1. Egress initializes the upstream HTTP/2 stream receive window with a size bounded by `control.transport.initial_download_credit_bytes` (default 8 MiB).
2. Egress reads HTTP/2 `DATA` frame bytes from the upstream stream and forwards them as NATS `DataFrame`s on `e2c` to Control.
3. Egress must **withhold** sending HTTP/2 `WINDOW_UPDATE` frames to the upstream origin if the NATS download credit for the request (granted by Control via `CreditFrame`) is exhausted.
4. When Control publishes a NATS `CreditFrame` to Egress, Egress increments the local credit counter and sends an HTTP/2 `WINDOW_UPDATE` frame to the upstream origin stream to allow the origin to resume downloading.

---

## 5. Pseudo-Header Normalization

HTTP/2 uses pseudo-headers (colon-prefixed, e.g., `:method`) instead of the HTTP/1.1 request line. Straw normalizes these at both ingress and egress boundaries.

### Ingress Normalization (Control-side)
1. Control maps HTTP/2 pseudo-headers to the internal request structure:
   - `:method` maps to the request HTTP method.
   - `:scheme` maps to the request scheme (`http` or `https`).
   - `:authority` maps to the `Host` header (or is used to populate it if `Host` is absent).
   - `:path` maps to the request path and query string.
2. Control strictly strips all other colon-prefixed headers from the headers collection before sending the request to the dispatcher and egress.

### Egress Normalization (Worker-side)
1. Egress translates the NATS `RequestStart` frame to an outbound HTTP/2 request:
   - `:method` is populated from the request method.
   - `:scheme` is populated from the target scheme.
   - `:path` is populated from the target path and query.
   - `:authority` is populated from the original request hostname (and optional port), preserving the SNI host. It must **never** be populated from the validated dial IP address.
2. Egress strips all incoming colon-prefixed headers from the request headers collection.
3. Egress rejects any client-supplied headers containing colon prefixes to prevent pseudo-header injection attacks.

---

## 6. Trailer Behavior

HTTP/2 supports trailers natively as a HEADERS frame at the end of the stream with the `END_STREAM` flag set.

### Outbound Trailers (Upstream to Egress to Control)
1. When Egress receives HTTP/2 trailers from the upstream origin, it packages them into a NATS `TrailersFrame` and publishes it on the `e2c` subject.
2. The `TrailersFrame` must be transmitted before the terminal `EndFrame`.

### Inbound Trailers (Control to Client)
1. When Control receives a NATS `TrailersFrame` from Egress, it writes the trailers to the client's HTTP/2 stream.
2. Since HTTP/2 natively supports trailers on all streams, Control does not drop trailers for HTTP/2 clients.

---

## 7. Connection-Level Error Fanout

Because multiple HTTP/2 streams share a single TCP connection, connection-level failures (e.g., TCP reset, TLS Alert, GOAWAY, Ping timeout) affect all concurrent in-flight requests.

### Egress Connection Failure
1. If a pooled or active HTTP/2 connection to an upstream origin fails, Egress must identify all active streams (requests) multiplexed on that connection.
2. For each active request:
   - Egress cancels the request context.
   - Egress publishes an `ErrorFrame` with code `upstream_reset` (or `upstream_connection_refused` if the failure occurred during handshake) on the request's `e2c` subject.
3. The failed connection is immediately evicted from the Egress connection pool, and no new streams may be initiated on it.
4. Egress ensures no goroutines or stream resources are leaked during connection teardown.

### Inbound Client Connection Failure
1. If a client HTTP/2 connection to Control fails, Control cancels all active request contexts associated with the connection.
2. Control publishes a NATS `CancelFrame` on each request's `c2e` subject.

---

## 8. MITM ALPN Behavior

When Man-In-The-Middle (MITM) decryption is active, Control terminates the client TLS connection and Egress initiates the upstream TLS connection. ALPN negotiation must match destination capabilities.

### ALPN Negotiation Rules
1. During the inbound TLS handshake with a client, Control offers `h2` and `http/1.1` in the ALPN extension only if `control.http2.enabled` is `true` and the tenant's policy permits HTTP/2.
2. If the client selects `h2`, Control accepts it and routes the requests as multiplexed streams.
3. If the client does not support `h2`, Control falls back to `http/1.1`.

### Protocol Translation
1. If Control negotiates `h2` with the client, but the Egress worker negotiates `http/1.1` with the upstream origin (due to origin lack of HTTP/2 support or disabled outbound flags), Straw must perform protocol translation:
   - Control forwards the request from the client's HTTP/2 stream.
   - Egress establishes a standard HTTP/1.1 outbound connection to the origin.
   - Response frames are translated from HTTP/1.1 back to HTTP/2 streams at Control.

---

## 9. Egress HTTP/1.1/HTTP/2 Downgrade Rules

Egress must transparently fallback to HTTP/1.1 when an upstream origin does not support HTTP/2 or explicitly requests a downgrade.

### ALPN Negotiation Fallback
1. When Egress performs an outbound TLS handshake with `egress.http2.enabled` set to `true`, it includes `h2` and `http/1.1` in the ALPN extension.
2. If the upstream origin negotiates `http/1.1` (or returns no ALPN), Egress transparently establishes an HTTP/1.1 transport for the request.
3. Egress caches the destination hostname as HTTP/1.1-only for a configurable duration (default 5 minutes) to bypass subsequent ALPN/HTTP2 attempts for the same host, avoiding roundtrip handshake penalties.

### Explicit Downgrade (`HTTP_1_1_REQUIRED`)
1. If Egress attempts to send an HTTP/2 request but receives an HTTP/2 `RST_STREAM` with error code `HTTP_1_1_REQUIRED` (0xd):
   - Egress immediately evicts the connection from the pool.
   - Egress caches the destination hostname as HTTP/1.1-only.
   - If the request is replayable (e.g., before `RequestStart` data was fully transmitted or `replayable=true`), Egress retries the request transparently over a new HTTP/1.1 connection.
   - If the request cannot be safely retried, Egress fails the request with `upstream_reset` and includes `"http_1_1_required"` in the diagnostics details.

---

## 10. Implementation Test Rows

Before any HTTP/2 code is merged, the following test coverage is required:

| Area | Required Checks | Owning Task |
|------|-----------------|-------------|
| Disabled Default | Default config disables HTTP/2 ALPN and falls back to HTTP/1.1 | `../implementation-history.md#p2-15` |
| Multiplexing | Multiple concurrent requests are sent over a single TCP connection as separate HTTP/2 streams | `../implementation-history.md#p2-15` |
| Stream Cancellation | Client stream cancel sends `CancelFrame`; NATS `CancelFrame` sends `RST_STREAM` with `CANCEL` | `../implementation-history.md#p2-15` |
| Error Mapping | All HTTP/2 `RST_STREAM` codes map to the correct Straw `ErrorCode`s | `../implementation-history.md#p2-15` |
| Flow Control backpressure | NATS credit exhaustion stops HTTP/2 window updates; credit replenishment sends `WINDOW_UPDATE` | `../implementation-history.md#p2-15` |
| Pseudo-headers | `:method`, `:scheme`, `:authority`, and `:path` are mapped correctly; Host maps to `:authority`; custom pseudo-headers are rejected | `../implementation-history.md#p2-15` |
| Trailers | Upstream HTTP/2 trailers are mapped to NATS `TrailersFrame` and sent to client | `../implementation-history.md#p2-15` |
| Connection-level Error | A connection failure aborts all multiplexed streams and publishes NATS `ErrorFrame`s | `../implementation-history.md#p2-15` |
| ALPN Negotiation | MITM correctly negotiates client ALPN based on config/policy | `../implementation-history.md#p2-16` |
| Protocol Translation | HTTP/2 ingress stream translated to HTTP/1.1 egress; response translated back | `../implementation-history.md#p2-16` |
| Downgrade / ALPN Fallback | Egress falls back to HTTP/1.1 when ALPN negotiation fails to select `h2` | `../implementation-history.md#p2-15` |
| Downgrade / HTTP_1_1_REQUIRED | Stream reset with `HTTP_1_1_REQUIRED` evicts connection, caches state, and downgrades request | `../implementation-history.md#p2-15` |

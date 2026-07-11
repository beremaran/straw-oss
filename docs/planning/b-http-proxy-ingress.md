# Appendix B - HTTP Proxy Ingress

This appendix defines the P1 HTTP forward proxy contract consumed by:

- `../implementation-history.md#p1-02`
- `../implementation-history.md#p1-03`
- `../implementation-history.md#p1-04`
- `../implementation-history.md#p1-05`
- `../implementation-history.md#p1-06`
- `../implementation-history.md#p1-18`

## Listener

The HTTP forward proxy listens on the P1 proxy port from Section 28, port `8081`, only when enabled by static Control
configuration. P0 REST/config/admin APIs stay on port `8080`.

The forward proxy accepts absolute-form HTTP/1.1 proxy requests for `http://` and `https://` upstream URLs. It rejects
`CONNECT`; raw CONNECT is owned by `../implementation-history.md#p1-05`.

## Authentication

Proxy clients carry the Straw API key in `Proxy-Authorization`:

```http
Proxy-Authorization: Bearer <api_key>
```

The proxy listener authenticates and authorizes the key with the same data-plane execution checks as
`POST /api/v1/requests`. `Authorization` is treated as an ordinary upstream header and is not used for Straw proxy
authentication.

Control must remove `Proxy-Authorization` before constructing the internal request model. It is never forwarded to
Egress, upstream origin servers, metadata rows, audit values, or logs.

Missing, malformed, invalid, revoked, or unauthorized proxy credentials fail before routing. Public errors are rendered
on the proxy socket using the rules below.

## Request Mapping

Each accepted proxy request maps to the same decoded internal request model used by the P0 REST transport, with these
fixed or derived values:

| Internal field | Proxy mapping |
|----------------|---------------|
| `method` | HTTP method token from the proxy request. `CONNECT` is rejected on the forward-proxy listener. |
| `url` | Absolute-form proxy request target. URL fragments, userinfo, empty host, and IPv6 zone identifiers are rejected. |
| `headers` | Incoming headers after stripping proxy-only, internal, hop-by-hop, and transport-computed headers. Order and duplicates are preserved for remaining headers. |
| `body` | Streamed from the client socket into the existing Control-to-Egress body frame path. No REST `inline_base64` envelope is exposed. |
| `routing.ingress_type` | `http_proxy`. |
| `routing` hints | No client-supplied routing hints are accepted in P1 proxy requests. |
| `fingerprint_profile` | Tenant default applies. Client-selected profiles are out of scope for P1 proxy ingress. |
| `timeout_ms` | Deployment or tenant default, capped by existing request deadline limits. |
| `replayable` | `true` for `GET`, `HEAD`, and `OPTIONS`; `false` for `PUT`, `DELETE`, `POST`, and `PATCH`. Control never changes a non-replayable request to replayable after acceptance. |
| `capture_hint` | `none`. Payload capture remains P2. |

The proxy listener applies the REST validation rules from Section 7 where they fit a raw proxy request:

- method is uppercase and known;
- URL is absolute HTTP or HTTPS;
- header names are valid HTTP tokens;
- header values contain no bare CR or LF;
- configured header count, header-name length, aggregate-header-byte, body, and timeout limits apply;
- client-supplied `Host` is not forwarded as an independent header; Egress derives outbound Host from the URL;
- `Content-Length`, `Transfer-Encoding`, and hop-by-hop headers are not forwarded as caller-controlled metadata.

The proxy listener also strips all `X-Straw-*` headers and every header named by the request's `Connection` header.

## Raw Socket Responses

HTTP proxy transport does not use the REST JSON success envelope. If Straw receives upstream status and headers, Control
writes the upstream HTTP status, headers, body bytes, and supported trailers directly to the proxy client. Upstream
`3xx`, `4xx`, and `5xx` statuses are normal successful Straw transports.

If Straw fails before writing upstream response headers, Control renders a plain HTTP response on the proxy socket:

- HTTP status comes from the canonical public error mapping, except `route_no_match` uses HTTP `421` for decoded proxy
  modes;
- `Content-Type` is `application/json`;
- the body is the canonical public `ErrorResponse` JSON;
- `Proxy-Authenticate: Bearer` is included only for authentication failures;
- internal error facts, worker identities, routing diagnostics, and policy bundle details are not exposed.

If an upstream or internal failure occurs after Control has written upstream response headers to the proxy client,
Control cannot replace the response with a JSON error. It must close the response body stream, record the terminal
outcome in metadata, and cancel the running request if the failure came from the client side.

## Streaming And Backpressure

Proxy responses stream from Egress to Control to the proxy client. Control must not buffer the whole response body for
proxy transport.

Backpressure is credit-based:

1. Control grants response-body credit only as it can write bytes to the proxy client.
2. Egress sends response `DataFrame`s only within available credit.
3. If the proxy client stops reading, Control stops granting credit and eventually cancels on deadline or idle timeout.
4. If the proxy client disconnects, Control sends `CancelFrame` and records a client-aborted terminal outcome.

The same total deadline, assignment timeout, connect timeout, response-header timeout, upload idle timeout, download
idle timeout, and frame idle timeout hierarchy from Section 9 applies.

## Trailers

The internal protocol may carry trailers. The HTTP forward proxy may forward trailers only when the downstream response
framing can legally carry them.

- HTTP/1.1 chunked downstream response: forward upstream trailers after applying the same header-name and CR/LF
  validation rules used for response headers.
- Fixed-length or connection-close downstream response: do not forward trailers; record trailer names and aggregate
  trailer byte count in request metadata, with sensitive values redacted.
- HTTP/2 downstream proxy semantics are out of scope until `../implementation-history.md#p2-14` specifies them.

`Transfer-Encoding`, `Content-Length`, `Connection`, `Proxy-Authorization`, and `X-Straw-*` trailers are never forwarded.

## Implementation Test Rows

Before P1 implementation starts, the consuming tasks must cover at least these rows:

| Area | Required checks | Owning task |
|------|-----------------|-------------|
| Proxy authentication | valid `Proxy-Authorization: Bearer`; missing credentials; malformed credentials; revoked or unauthorized key; `Authorization` forwarded only as an upstream header | `../implementation-history.md#p1-02` |
| Header sanitization | `Proxy-Authorization`, `X-Straw-*`, hop-by-hop headers, `Connection`-named headers, caller `Host`, `Content-Length`, and `Transfer-Encoding` do not reach Egress as forwarded headers | `../implementation-history.md#p1-02` |
| Request mapping | absolute-form HTTP and HTTPS targets become decoded requests; invalid method, fragment, userinfo, empty host, IPv6 zone ID, bad header name, and bare CR/LF are rejected before routing | `../implementation-history.md#p1-02` |
| Routing input | proxy requests set `ingress_type=http_proxy`; route matching can distinguish `rest`, `http_proxy`, `connect`, and `mitm`; incompatible workers are ineligible | `../implementation-history.md#p1-04` |
| Raw response rendering | upstream `2xx`, `3xx`, `4xx`, and `5xx` stream without JSON envelopes; pre-header Straw errors render canonical JSON error bodies with proxy HTTP status rules | `../implementation-history.md#p1-03` |
| Post-header failure | upstream or transport failure after headers closes the stream, records terminal metadata, and does not attempt a second HTTP response | `../implementation-history.md#p1-03` |
| Backpressure | a slow client prevents unbounded Control buffering; credit is withheld until writes progress; deadline or idle timeout cancels stalled streams | `../implementation-history.md#p1-03` and `../implementation-history.md#p1-18` |
| Client cancellation | proxy client disconnect sends `CancelFrame` and releases request-scoped resources | `../implementation-history.md#p1-03` |
| Trailers | chunked downstream responses forward allowed trailers; non-trailer-capable responses metadata-capture trailer names/byte counts and drop values according to redaction rules | `../implementation-history.md#p1-03` |
| CONNECT separation | forward proxy listener rejects `CONNECT`; raw tunnel behavior is implemented only by the CONNECT task | `../implementation-history.md#p1-05` |

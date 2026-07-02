## 15. HTTP Semantics

### Origin Status Passthrough

Origin 3xx, 4xx, and 5xx responses are normal upstream responses if Straw received upstream status/headers. They are
logged as outcome metadata but are not converted into Straw errors.

For P0 REST, Straw API HTTP status is `200` for successful transport. Upstream status is carried in the JSON response
envelope as `status`.

**Client note**: Clients must inspect the JSON envelope's `status` field to determine the upstream HTTP result. The
outer HTTP status only describes Straw API transport success or failure.

### Methods

P0 REST decoded transport accepts standard HTTP methods except `CONNECT`. Raw CONNECT is only accepted by the P1 CONNECT
ingress.

Automatic Control fallback after `RequestStart` is disabled unless `replayable=true`.

### Headers

Control strips internal Straw routing/control headers before dispatch. `Proxy-Authorization` is never forwarded
outbound. Header order and duplicates are preserved.

### Cookies

Cookies pass through as headers. Straw does not maintain cookie jars.

### Header Injection

P0 supports only explicit Control-resolved header operations from configured injection policy. The operation list sent
to Egress is ordered and bounded.

Allowed P0 operations:

- set header,
- append header,
- remove header.

P0 does not support live body mutation, JavaScript mutation, cookie-jar persistence, or content-aware rewriting.

**Injection safety rules**:

| Header              | Injection Rule                                                   |
|---------------------|------------------------------------------------------------------|
| `Host`              | Deny, unless explicitly supported by deployment config           |
| `Content-Length`    | Deny (computed by Egress)                                        |
| `Transfer-Encoding` | Deny                                                             |
| `Connection`        | Deny                                                             |
| `Proxy-Authorization` | Deny                                                           |
| `X-Straw-*`         | Deny                                                             |
| `Authorization`     | Allow only if tenant_admin-created policy; audit-redacted         |
| `Cookie`            | Allow only if tenant_admin-created policy; audit-redacted         |

All header name matching is case-insensitive. Duplicate `set` operations for the same header are rejected. `append` may repeat a header name. Maximum
injected header bytes is bounded by `control.transport.max_frame_data_bytes`. Injected header values must not contain
bare CR or LF characters.

### Redirects

Egress does not follow redirects in P0. Redirect responses pass through as upstream responses. Redirect following
requires explicit request flag and tenant policy in a later phase because it changes request count, destination, and
cost. Any future redirect-following implementation must re-run Control-equivalent host policy and Egress resolved-IP
policy on every redirect target.

### Compression

Egress preserves upstream `Content-Encoding`. It does not decode or recompress in P0/P1. Payload capture of compressed
bodies in P2 stores raw compressed bytes unless an explicit decompression capture feature is added later.

### Trailers

The internal protocol supports trailers. Ingress-specific support depends on the public protocol implementation. If an
ingress cannot send trailers to the client, Control records that limitation and drops/metadata-captures trailers
according to that ingress contract.

### HTTP/2

P0 does not support HTTP/2 semantics. P0 Egress disables outbound HTTP/2 by default.

P2 may add HTTP/2 after defining:

- one `request_id` per HTTP/2 stream,
- stream cancellation mapping,
- flow-control interaction with NATS credit,
- pseudo-header normalization,
- trailer behavior,
- connection-level error fanout,
- MITM ALPN behavior,
- egress HTTP/1.1/HTTP/2 downgrade rules.

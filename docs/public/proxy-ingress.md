# HTTP and HTTPS proxy ingress

Control's API listener is also a standard forward-proxy endpoint. It accepts absolute-form HTTP requests and
HTTP/1.1 `CONNECT` tunnels on the same `server.api_port` used by the REST API. The official Egress worker supports
both modes by default.

## Send proxied requests

With the local stack running on port 8080:

```sh
curl --proxy http://localhost:8080 http://example.com
curl --proxy http://localhost:8080 https://example.com
```

The HTTP request is decoded by Control, dispatched over NATS, executed by Egress, and streamed back as an ordinary
HTTP response. For an HTTPS URL, curl sends `CONNECT example.com:443`; Egress opens the policy-checked TCP
connection, then Control returns `200 Connection Established` and relays opaque bytes in both directions.

When `STRAW_AUTH_TOKEN` is set, authenticate to the proxy with `Proxy-Authorization`, not the destination's
`Authorization` header:

```sh
curl --proxy http://localhost:8080 \
  --proxy-header 'Proxy-Authorization: Bearer <token>' \
  https://example.com
```

Control removes proxy credentials and hop-by-hop headers before dispatch. An ordinary `Authorization` header on an
absolute-form HTTP request remains an end-destination header.

## Behavior and limits

- Absolute-form requests may use `http://` or `https://` URLs and the REST API's supported methods except `CONNECT`.
- Proxy request bodies use `request.max_inline_request_body_bytes`. Responses stream directly and are not limited by
  `max_inline_response_body_bytes` or eligible for response receipts.
- The deployment default timeout applies to proxy requests and tunnels, bounded by `request.max_timeout_ms`. A
  CONNECT tunnel closes when that deadline, cancellation, a stream idle timeout, or either endpoint ends it.
- Destination hostname, resolved-address, and network policy checks are the same checks used by the REST ingress.
- Routing sees `ingress_type: "http_proxy"` for decoded proxy requests and `ingress_type: "connect"` for tunnels.
  An explicitly configured worker `supported_ingress_modes` list must include the needed value.
- CONNECT is an opaque TCP tunnel. Straw does not intercept TLS, inspect tunneled HTTP, inject headers, or apply an
  outbound TLS fingerprint profile to the client's TLS session.
- CONNECT requires an explicit `host:port` authority and HTTP/1.1 connection hijacking. Extended CONNECT over HTTP/2,
  UDP tunneling, and SOCKS are not supported.

Before the CONNECT success response, failures use the canonical JSON error body with an HTTP status. After Control
has returned `200 Connection Established`, a later failure closes the tunnel because another HTTP response cannot be
sent inside the established byte stream.

## Expose the proxy safely

The proxy and REST API share a listener and deployment-wide credential. Anyone who can reach that listener and holds
the token can make requests allowed by the deployment policy. Put Control behind TLS and network access controls,
and never expose an unauthenticated listener to an untrusted network. See [Security](security.md) and
[Deployment patterns](deployment.md).

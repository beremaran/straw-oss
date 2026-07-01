# TODO

## Browser-compatible proxy support

### CONNECT tunnel

- Add a proxy listener on Control for browser `CONNECT host:port` requests.
- Keep the existing REST `/v1/request` endpoint unchanged.
- Create a tunnel protocol over NATS:
  - open tunnel from Control to selected Egress
  - report open success/failure back to Control
  - stream client-to-egress byte chunks
  - stream egress-to-client byte chunks
  - close tunnels from either side with a reason
- Validate `CONNECT` targets with the same private-IP and URL safety rules used by REST requests.
- Decide tunnel limits:
  - idle timeout
  - max tunnel lifetime
  - chunk size
  - max in-flight chunks/backpressure
  - max concurrent tunnels
- Handle disconnects and cleanup:
  - browser closes connection
  - Egress TCP dial fails
  - target closes connection
  - NATS publish/subscribe fails
  - Control or Egress shuts down
- Decide whether to support only `CONNECT` initially, or also plain HTTP proxy requests.

### MITM flows

- Treat MITM as a separate mode from normal proxy tunneling.
- Add a Control-side root CA story:
  - generate/import CA
  - store private key securely
  - install/trust CA in browsers or OS
  - rotate/revoke CA
- Generate per-host leaf certificates after `CONNECT`.
- Terminate browser TLS at Control, then forward decrypted requests through the existing REST-like request/response path.
- Decide HTTP protocol support:
  - force HTTP/1.1 only
  - or support HTTP/2 multiplexing correctly
- Handle streaming response bodies, WebSockets, SSE, and large uploads.
- Document unsupported cases:
  - certificate pinning
  - mTLS sites
  - clients that do not trust the Straw CA
- Add explicit security controls because Control can see cookies, credentials, request bodies, and response bodies.

## Parameters and authentication

- Define how clients select or constrain Egress:
  - fixed Control `egress_id`
  - per-request header
  - proxy username field
  - query/config profile
- Decide which request parameters apply to both REST and proxy listeners:
  - target timeout
  - max response size
  - max body size
  - allowed ports
  - allow/disallow private IPs
  - egress selection
- Add shared authentication for REST and proxy listeners:
  - REST: `Authorization` header or existing gateway auth
  - proxy: `Proxy-Authorization`
  - same token/credential validation path where possible
- Decide whether REST and proxy share one listener/port or use separate ports.
- Decide whether REST and proxy share the same rate/concurrency limits or have separate limits.
- Make audit logs consistent across REST, CONNECT tunnels, and MITM:
  - request/tunnel ID
  - client identity
  - selected Egress
  - target host
  - status/error
  - bytes transferred for tunnels

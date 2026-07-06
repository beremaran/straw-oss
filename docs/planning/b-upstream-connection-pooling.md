# Appendix B - Upstream Connection Pooling

This appendix defines the P1 upstream connection-pooling contract consumed by:

- `docs/tasks/p1/16-upstream-connection-pooling.md`

Pooling is optional, disabled by default, and never required for request correctness.

## Feature Flag And Config

The shipped default remains one upstream connection per request. Enabling pooling requires an explicit Egress static
configuration flag:

| Config key | Default | Rule |
|------------|---------|------|
| `egress.upstream_connection_pool.enabled` | `false` | When `false`, Egress keeps the P0 transport behavior: outbound HTTP/2 disabled and upstream keep-alives disabled. |
| `egress.upstream_connection_pool.max_idle_conns_per_tenant_host` | `2` | Maximum idle direct-local connections for one tenant, scheme, hostname, port, resolved IP, and fingerprint profile. |
| `egress.upstream_connection_pool.idle_timeout_ms` | `30000` | Idle pooled connections close after this timeout. |
| `egress.upstream_connection_pool.max_lifetime_ms` | `300000` | Connections close after this lifetime even if still reusable. |

Implementations must reject enabled pooling unless focused tests cover the rows in Section 30.

## Pool Boundary

Connections may be reused only inside an exact pool key:

- tenant ID;
- destination resolution mode;
- scheme;
- original hostname;
- port;
- validated dial IP;
- fingerprint profile.

Direct-local pooling must not share connections across tenants, hosts, ports, resolved IPs, or fingerprint profiles. A
connection that receives `Connection: close`, an HTTP protocol error, a TLS error, or any response-body read failure is
not returned to the pool.

Upstream proxy remote resolution keeps the P0 no-reuse behavior; this P1 pooling spec is direct-local only.

## SSRF Invariant

Pooling preserves the direct-local invariant from Section 16 and Section 27:

1. Resolve the original hostname for the current request.
2. Validate the full resolved IP set against the request's `DestinationPolicy`.
3. Reuse only an idle connection whose dial IP is present in the validated set for the current request.
4. Otherwise dial one of the just-validated IPs directly.

The HTTP/TLS library must not perform a second DNS lookup. SNI, certificate verification, and the outbound `Host` value
remain bound to the original request hostname, not the dial IP.

Pooling cannot be used to skip DNS or destination-policy validation for a later request. If validation fails, the
request fails even when a matching idle connection exists.

## Eviction And Shutdown

Egress closes pooled connections when:

- idle timeout expires;
- maximum lifetime expires;
- tenant or worker shutdown starts;
- the connection's validated IP is not present in the current request's validated IP set;
- the connection observes a protocol, TLS, body-read, or upstream reset error.

Shutdown must stop admitting new pooled connections, close idle connections immediately, and let active requests finish
until their normal deadlines.

## Observability And Failure Modes

Pooling metrics and logs must use bounded labels only: tenant ID, pool enabled/disabled, reuse/miss/eviction reason, and
resolution mode. They must not include request IDs, full URLs, header values, credentials, or raw NATS subjects.

Pool exhaustion, stale connections, or idle connection reuse failure must fall back to a fresh validated dial for the
same request when the request deadline still allows it. They must not change routing, retry across workers, or make the
request depend on pooling for success.

## Implementation Test Rows

Before code implementation starts, the consuming task must cover at least these rows:

| Area | Required checks | Owning task |
|------|-----------------|-------------|
| Disabled default | default config keeps `DisableKeepAlives=true`, outbound HTTP/2 disabled, and opens no reusable upstream connection | `docs/tasks/p1/26-upstream-connection-pooling-implementation.md` |
| Enabled reuse | enabled config reuses a connection only for the same tenant, scheme, hostname, port, validated IP, and fingerprint profile | `docs/tasks/p1/26-upstream-connection-pooling-implementation.md` |
| Tenant isolation | two tenants targeting the same host never share a pooled connection | `docs/tasks/p1/26-upstream-connection-pooling-implementation.md` |
| DNS rebinding and SSRF | every request resolves and validates before reuse; a pooled connection is discarded when its dial IP is absent from the current validated set | `docs/tasks/p1/26-upstream-connection-pooling-implementation.md` |
| Second-resolution guard | the outbound transport dials only validated IPs and does not let the HTTP/TLS library resolve the hostname independently | `docs/tasks/p1/26-upstream-connection-pooling-implementation.md` |
| Eviction and shutdown | idle timeout, max lifetime, protocol/TLS/body errors, and worker shutdown close reusable connections without leaking goroutines | `docs/tasks/p1/26-upstream-connection-pooling-implementation.md` |
| Failure fallback | stale or closed pooled connections fall back to a fresh validated dial when deadline permits and surface canonical errors when it does not | `docs/tasks/p1/26-upstream-connection-pooling-implementation.md` |

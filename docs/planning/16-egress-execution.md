## 16. Egress Execution

### Outbound TLS

Egress uses `bogdanfinn/tls-client` or another configured outbound TLS implementation to apply browser-like outbound TLS
fingerprints. This is outbound-client behavior only.

Control never asks Egress to guess a fingerprint. Control sends a supported enum/profile. If unsupported, Egress rejects
or fails with `unsupported_fingerprint`.

### P0 Transport Defaults

P0 Egress disables outbound HTTP/2 and upstream keep-alives by default to avoid promising HTTP/2 or connection-pool
semantics before they are specified and tested.

P0 may still reuse local process objects such as DNS resolvers, TLS profile definitions, and bounded worker pools. It
must not rely on cross-request upstream connection reuse for correctness or performance claims.

### CGO Isolation

If the outbound TLS stack uses CGO/FFI, the worker isolates it from the NATS message loop using bounded worker pools and
deadline-aware execution. The NATS receiver must remain responsive under high outbound TLS load.

### DNS and Deny Enforcement

Egress validates destination policy immediately before connect using the resolved IP set and the `DestinationPolicy`
bundle included in `RequestStart`.

Egress must block unless explicitly allowed by policy:

- private RFC1918 ranges,
- loopback,
- link-local,
- multicast,
- metadata service IPs such as cloud instance metadata addresses,
- denied CIDRs,
- denied resolved CNAME targets,
- redirect destinations that violate policy,
- SNI/Host mismatch when policy forbids it.

Control performs pre-routing URL/host validation. Egress performs final resolved-IP validation because DNS resolution
occurs closest to the outbound connection. Workers do not query Postgres, Redis, or ClickHouse to obtain destination
policy.

### Redirect Handling

P0 does not follow redirects. Future redirect following must validate every redirect target at both boundaries:

- Control-equivalent URL/host policy before the next request,
- Egress resolved-IP policy immediately before connect.

### Error Reporting Boundary

Egress reports constrained low-level failure facts. Control maps facts to public ErrorCode and HTTP status.

Examples:

| Egress fact                       | Control public code                    |
|-----------------------------------|----------------------------------------|
| `dns_no_records`                  | `upstream_dns_failure`                 |
| `dns_denied_ip`                   | `destination_denied`                   |
| `tcp_refused`                     | `upstream_connection_refused`          |
| `tls_handshake_failed`            | `upstream_tls_failure`                 |
| `deadline_exceeded_connect`       | `timeout_exceeded` + `CONNECT_TIMEOUT` |
| `upstream_reset_before_headers`   | `upstream_reset`                       |
| `unsupported_fingerprint_profile` | `unsupported_fingerprint`              |

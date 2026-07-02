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

### DNS Validation and Dial Target Invariant

Egress validates destination policy immediately before connect using the resolved IP set and the `DestinationPolicy`
bundle included in `RequestStart`.

**Dial target invariant (direct execution)**: The resolver, destination-policy validator, and dialer are one unit.
Egress must connect **only** to an IP address that passed policy validation for the current request. Egress must not
validate one DNS result and then allow the HTTP/TLS library to perform an independent second resolution.

For HTTPS, the dial target can be the validated IP, while SNI and certificate verification remain bound to the original
hostname. The original `Host` header, SNI host, and certificate verification must all use the original request hostname.

**Upstream proxy mode**: When an upstream proxy is configured, Egress cannot prove the resolved-IP policy because the
proxy performs DNS resolution and connection establishment. In this mode:

- The destination policy mode is `DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE`.
- Egress validates the proxy address itself against destination deny rules.
- The proxy is trusted to enforce equivalent destination policy per deployment configuration.
- If the deployment does not trust the proxy for SSRF enforcement, requests using this mode are rejected with
  `destination_denied`.

**Provider adapter mode**: The adapter must enforce equivalent destination policy and report constrained facts back to
Control.

Egress must block unless explicitly allowed by policy:

- private RFC1918 ranges,
- loopback (`127.0.0.0/8`, `::1/128`),
- link-local (`169.254.0.0/16`, `fe80::/10`),
- multicast (`224.0.0.0/4`, `ff00::/8`),
- metadata service IPs (`169.254.169.254`, `169.254.169.253`, `100.100.100.200`, and cloud provider
  metadata ranges),
- documentation ranges (`0.0.0.0/8`, `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`, `198.18.0.0/15`),
- reserved IPv6 (`fc00::/7`, `fe80::/10`, `ff00::/8`, `::ffff:0:0/96` for IPv4-mapped),
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

Egress maps low-level failure facts to canonical `ErrorCode` values before emitting `ErrorFrame`. The frame carries
the code directly plus the originating fact in `details["fact"]` for diagnostics. Control validates the code against
the executor-emittable set (Section 13), owns the final HTTP status/category/retryability mapping from the canonical
error registry, and sanitizes all executor-supplied text before anything reaches a client.

Fact-to-code mapping applied by Egress:

| Egress fact (`details["fact"]`)   | Emitted `ErrorCode`                    |
|-----------------------------------|----------------------------------------|
| `dns_no_records`                  | `upstream_dns_failure`                 |
| `dns_denied_ip`                   | `destination_denied`                   |
| `tcp_refused`                     | `upstream_connection_refused`          |
| `tls_handshake_failed`            | `upstream_tls_failure`                 |
| `deadline_exceeded_connect`       | `timeout_exceeded` + `CONNECT_TIMEOUT` |
| `upstream_reset_before_headers`   | `upstream_reset`                       |
| `unsupported_fingerprint_profile` | `unsupported_fingerprint`              |

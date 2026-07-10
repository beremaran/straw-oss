## 27. Security Controls

### API Key Storage and Generation

API keys are stored as secure hashes, not plaintext. A visible key prefix may be stored for identification. Revocation
updates Postgres, increments tenant config version, and publishes invalidation.

API key generation requirements:

- API keys must contain at least 128 bits of entropy; 192 or 256 bits preferred.
- Lookup uses visible prefix to find candidates, then constant-time hash comparison.
- Prefix collisions must be handled by checking all candidates with the same prefix.
- Server-side pepper is supported and loaded from secret manager or environment variable.
- Key material is shown only once at creation time.

Use HMAC-SHA-256 with a server-side pepper or Argon2id for key hashing. Plain SHA-256 is acceptable only if keys are
high-entropy random tokens and never user-chosen.

### Worker Credential Signing

Worker credentials use Ed25519.

The worker signs a registration token containing:

- `credential_id`,
- `worker_id`,
- `tenant_scope`,
- `pool_scope`,
- `nonce`,
- issued-at timestamp,
- protocol version.

For protocol minor `1`, the canonical signed payload additionally contains a count and length-prefixed, deterministically
sorted unique `supported_fingerprint_profiles` list. Minor `0` and empty-list registrations preserve the legacy bytes.
Control validates the list against the exact catalog and credential subset before creating the immutable runtime session;
the Egress registry remains the final execution authority.

Control verifies signature using stored public key and rejects stale timestamps/nonces according to policy.

**Registration nonce replay protection**: Nonces are stored in Redis with TTL, scoped by `credential_id`. If Redis is
unavailable, registration fails closed unless deployment explicitly allows fail-open worker registration. Fail-open
worker registration is not a recommended default. Clock skew tolerance is configurable; default is 60 seconds. Nonces
expire after their TTL and are never reused.

### Destination Deny Normalization and CIDR Defaults

Deny-rule evaluation must normalize:

- lowercase hostnames,
- IDNA/punycode,
- trailing dots,
- default ports,
- redirects in phases where redirects are followed,
- CNAME chains,
- IPv4 and IPv6 literals,
- IPv4-mapped IPv6,
- SNI vs Host mismatches,
- CONNECT target host/port.

**Default denied CIDR set (IPv4)**:

- `0.0.0.0/8` (current network)
- `10.0.0.0/8` (RFC1918 private)
- `100.64.0.0/10` (CGNAT)
- `127.0.0.0/8` (loopback)
- `169.254.0.0/16` (link-local)
- `172.16.0.0/12` (RFC1918 private)
- `192.0.0.0/24` (IETF protocol)
- `192.0.2.0/24` (documentation TEST-NET-1)
- `192.88.99.0/24` (6to4 relay)
- `192.168.0.0/16` (RFC1918 private)
- `198.18.0.0/15` (benchmarking)
- `198.51.100.0/24` (documentation TEST-NET-2)
- `203.0.113.0/24` (documentation TEST-NET-3)
- `224.0.0.0/4` (multicast)
- `240.0.0.0/4` (reserved)
- `255.255.255.255/32` (broadcast)

**Default denied CIDR set (IPv6)**:

- `::1/128` (loopback)
- `::/128` (unspecified)
- `::ffff:0:0/96` (IPv4-mapped)
- `64:ff9b::/96` (IANA IPv4-IPv6 translation prefix)
- `100::/64` (IANA discard-only prefix)
- `fc00::/7` (ULA)
- `fe80::/10` (link-local)
- `ff00::/8` (multicast)

**Cloud metadata IPs** (denied by default):

- `169.254.169.254` (AWS)
- `169.254.169.253` (AWS secondary)
- `169.254.170.2` (AWS credential endpoint)
- `100.100.100.200` (Alibaba Cloud)
- `100.100.100.201` (Alibaba Cloud metadata)

Private/link-local/metadata IP blocks are denied by default unless a tenant admin explicitly allows them for a tenant or
deployment.

Named `chrome_120` execution is a request-scoped `tls-client` client with explicit timeout, redirect, HTTP/3, dialer,
and cleanup controls. It retains the original hostname for SNI/certificate verification while dialing only the
validated IP. Unsupported profile instructions fail closed before DNS, and baseline requests retain the existing
transport path.

### SSRF Enforcement by Resolution Mode

Egress enforces destination policy based on the `DestinationResolutionMode` from `RequestStart`:

**Direct local resolution** (`DESTINATION_RESOLUTION_DIRECT_LOCAL`):
- Egress resolves the hostname, validates all resolved IPs against the deny list.
- Egress connects to the exact validated IP. The resolver, validator, and dialer are one unit.
- No second resolution is allowed by the HTTP/TLS library.

**Upstream proxy remote resolution** (`DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE`):
- Egress validates the proxy address against deny rules.
- Egress cannot prove the proxy's resolved-IP policy.
- Allowed only if the deployment trusts the proxy for equivalent SSRF enforcement.
- If not trusted, the request is rejected at Control before dispatch.

**Executor-delegated resolution** (`DESTINATION_RESOLUTION_EXECUTOR_DELEGATED`):
- A custom Egress implementation that resolves destinations internally must enforce equivalent destination policy.
- The implementation reports constrained facts back to Control.

### Metadata and Log Redaction

P0 metadata redaction is mandatory even though payload capture is not in P0.

- URL userinfo is rejected.
- Query strings are dropped from `target_url` by default.
- Authorization-like headers and cookie headers are never logged or written to ClickHouse in P0.
- Worker IDs and session IDs are internal-only and must not appear in public ErrorResponse objects.
- NATS subjects, credentials, signed URLs, private keys, and upstream proxy credentials are never written to logs except
  as redacted placeholders.
- API key secrets never appear in logs, audit events, ClickHouse records, or ErrorResponse details.

### Header Stripping

These are never forwarded unless explicitly documented otherwise:

- `Proxy-Authorization`,
- `X-Straw-*`,
- hop-by-hop headers invalid for the outbound protocol,
- internal trace headers unless injection policy allows propagation.

### Header Injection Safety Rules

P0 injection operations are validated before being sent to Egress. The canonical injection safety table (denied
headers, `tenant_admin`-only sensitive headers, case-insensitivity, duplicate-`set` rejection, size bounds, CR/LF
rules, and operator restrictions) lives in Section 15 — HTTP Semantics. This section does not restate it to avoid
drift.

### NATS Subject Tokens

`request_id`, `worker_id`, and `session_id` must be safe subject tokens:

- non-empty,
- bounded length,
- ASCII alphanumeric plus `_` and `-`,
- no dots,
- no wildcards,
- no path separators,
- no whitespace.

Invalid subject tokens are rejected before any NATS subscription or publish uses them.

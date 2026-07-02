## 27. Security Controls

### API Key Storage

API keys are stored as secure hashes, not plaintext. A visible key prefix may be stored for identification. Revocation
updates Postgres, increments tenant config version, and publishes invalidation.

Use an appropriate password/key hashing strategy for API tokens. Plain SHA-256 is acceptable only if keys are
high-entropy random tokens and never user-chosen. Prefer HMAC-SHA-256 with a server-side pepper or Argon2id if keys are
shorter or user-derived.

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

Control verifies signature using stored public key and rejects stale timestamps/nonces according to policy.

### Destination Deny Normalization

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

Private/link-local/metadata IP blocks are denied by default unless a tenant admin explicitly allows them for a tenant or
deployment.

### Metadata and Log Redaction

P0 metadata redaction is mandatory even though payload capture is not in P0.

- URL userinfo is rejected.
- Query strings are dropped from `target_url` by default.
- Authorization-like headers and cookie headers are never logged or written to ClickHouse in P0.
- Worker IDs and session IDs are internal-only and must not appear in public ErrorResponse objects.
- NATS subjects, credentials, signed URLs, private keys, and upstream proxy credentials are never written to logs except
  as redacted placeholders.

### Header Stripping

These are never forwarded unless explicitly documented otherwise:

- `Proxy-Authorization`,
- `X-Straw-*`,
- hop-by-hop headers invalid for the outbound protocol,
- internal trace headers unless injection policy allows propagation.

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

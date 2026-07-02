## 17. MITM Design — P2

MITM is not in P0. When implemented, it uses the same decoded internal request model as REST and HTTP proxy.

### TLS Library Boundary

Inbound TLS termination is server-side TLS. It should use Go `crypto/tls` or another server-capable TLS implementation.
Outbound `tls-client` is not assumed to support inbound server-side TLS termination.

MITM does not change the client's JA3/JA4 fingerprint. The client's fingerprint is produced by the client's TLS stack.
Control's server TLS configuration can affect compatibility, ALPN, supported versions, and certificates, but it cannot
make an inbound client appear like a different browser client.

### Certificate Terms

- Straw CA: operator-provided CA certificate/private key used to sign generated leaf certificates.
- Generated per-SNI certificate: leaf certificate.
- Intermediate CA: a signing CA below root; Straw does not generate per-SNI intermediate CAs.

### CA Handling

Operators provide CA material through static config. Straw may provide offline helper scripts to generate dev/test CA
material.

Control exposes the public CA certificate at `/api/v1/mitm/ca.pem` to authenticated users allowed to use MITM. Tenant
admins
configure and rotate the CA.

### Leaf Certificate Storage

Generated leaf cert storage policy must be explicit:

| Item                   | P2 policy                                                                     |
|------------------------|-------------------------------------------------------------------------------|
| Leaf cert public bytes | cacheable                                                                     |
| Leaf private key       | generated per SNI; stored only if encrypted at rest or disabled by config     |
| Redis cache            | encrypted serialized cert bundle or public cert only, depending on key policy |
| Disk cache             | optional local cache, encrypted when private key included                     |
| Object storage         | optional shared cache, encrypted, tenant/deployment scoped                    |
| TTL                    | no longer than configured `cert_validity_days`; default 30 days recommended   |
| Access                 | Control process only                                                          |

If private keys are not stored, Control regenerates leaf keypairs on cache miss. If private keys are stored, they must
be encrypted using a deployment key or KMS-compatible mechanism.

### Cache Miss Coalescing

Control uses:

- local singleflight per instance,
- Redis distributed lock across instances,
- bounded generation concurrency,
- CPU protection for unique-SNI floods.

A flood of unique SNIs bypasses singleflight deduplication, so Control must enforce a per-tenant and global
certificate-generation concurrency/rate limit.

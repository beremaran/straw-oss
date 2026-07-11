# Appendix C - MITM Leaf Certificate Design

This appendix defines the P2 MITM leaf-certificate policy consumed by:

- `../implementation-history.md#p2-01`
- `../implementation-history.md#p2-02`
- `../implementation-history.md#p2-03`
- `../implementation-history.md#p2-04`
- `../implementation-history.md#p2-20`

MITM leaf generation is disabled unless MITM is enabled for the deployment and tenant. Control is the only Straw
process that can generate, read, decrypt, or serve generated leaf certificate bundles.

Leaf generation and cache lookup require authenticated tenant identity before any cache key, KMS additional
authenticated data, or per-tenant flood limit is evaluated. In explicit-proxy MITM, Control authenticates the CONNECT
request first, then performs the inner server-side TLS handshake for the CONNECT authority with that authenticated
tenant captured in the certificate-generation path. The SNI remains an input to host validation and cache scoping, but
it is not a tenant identity source.

`deployment_id` is the stable configured Control deployment identity (`control.deployment_id` /
`STRAW_CONTROL_DEPLOYMENT_ID`). It is required when encrypted MITM leaf-bundle storage is enabled so cached private keys
cannot be replayed across deployments.

---

## 1. Resolved Key-Storage Policy

The resolved P2 policy is KMS-backed shared cache:

- Control generates one leaf private key and certificate per tenant/deployment/SNI.
- The generated bundle includes the public certificate chain and private key.
- Stored bundles are encrypted through a KMS-compatible mechanism before they leave Control memory.
- Cache entries are tenant scoped and deployment scoped.
- Only Control instances with the configured decrypt permission can read cached private keys.

Rejected options:

- Never storing generated leaf private keys. This avoids key-at-rest risk but forces regeneration on every cache miss
  and makes multi-Control cache sharing weaker.
- Encrypting Redis or disk cache with a static deployment key. This is simpler but weaker operationally than
  KMS-backed access control and rotation.

## 2. Storage Behavior

Leaf public certificate bytes are cacheable. Leaf private keys are cacheable only inside encrypted bundles.

| Storage | Allowed contents | Rule |
|---------|------------------|------|
| Redis | encrypted serialized bundle, or public certificate bytes only during diagnostics | Every key has a TTL and includes tenant/deployment/SNI scope. |
| Disk | optional encrypted local cache | Disabled by default; if enabled, private keys are present only in encrypted bundles. |
| Object storage | optional encrypted shared cache | Tenant/deployment scoped and encrypted before upload. |

Plaintext private keys must not be written to Redis, disk, object storage, logs, ClickHouse, config audit rows, or
error responses.

## 3. TTL And Rotation

Each generated certificate has `not_before`, `not_after`, and `cert_validity_days`.

- Cache TTL must never exceed the certificate's remaining validity.
- Configured TTL must be capped at `cert_validity_days`.
- Default `cert_validity_days` is 30 days.
- Control should refresh a cached bundle before expiry when the remaining validity is below the rotation window.
- CA rotation invalidates cached bundles signed by the old CA unless the deployment explicitly keeps the old CA trusted
  during a bounded overlap window.
- KMS key rotation must re-encrypt stored bundles or let them expire before disabling the old decrypt key.

## 4. Cache Miss Coalescing

A cache miss for one tenant/deployment/SNI uses all of these controls:

1. Local singleflight per Control instance.
2. Redis distributed lock across Control instances.
3. Bounded certificate-generation concurrency per Control process.
4. Per-tenant unique-SNI generation rate and concurrency limits.
5. Global unique-SNI generation rate and concurrency limits.

The Redis lock key must have a short TTL so a crashed Control instance cannot block generation indefinitely. Lock loss
does not permit plaintext key storage or bypass tenant/global limits.

Unique-SNI flood protection is required because singleflight only deduplicates repeated SNIs. When limits are exceeded,
Control fails the MITM request with a canonical overload/rate-limit error without generating a certificate.

A direct TLS listener whose `GetCertificate` callback only receives ClientHello/SNI must not use a global, placeholder,
or SNI-derived tenant in cache keys or KMS AAD. That path must be replaced by the authenticated CONNECT bootstrap before
tenant-scoped cache storage ships, or it must fail closed without generating cached tenant-scoped bundles.

That runtime replacement is owned by `../implementation-history.md#p2-04`; encrypted cache storage
and flood controls are owned by `../implementation-history.md#p2-20`.

## 5. Required Tests Before Implementation Ships

Implementation tasks must prove:

- cache miss generation stores an encrypted bundle and serves the generated leaf;
- cache hit decrypts the bundle and does not regenerate the keypair;
- stored private keys are encrypted at rest in Redis, disk cache if enabled, and object storage if enabled;
- cache TTL is capped by certificate validity;
- CA rotation invalidates old generated leaves;
- KMS/deployment key rotation preserves decryptability during the overlap and removes old-key dependence afterward;
- local singleflight collapses concurrent same-SNI misses;
- Redis lock collapses cross-Control same-SNI misses;
- bounded generation concurrency is enforced;
- per-tenant and global unique-SNI flood limits reject excess unique names without generation.

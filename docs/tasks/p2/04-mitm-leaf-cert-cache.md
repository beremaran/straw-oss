# 04 - MITM Leaf Certificate Cache

Status: not started

## Objective

Implement leaf certificate generation, cache/storage, coalescing, and flood controls exactly as specified by task 01.

## Required Planning Docs

- `docs/planning/17-mitm-design-p2.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/33-risks.md`

## Prerequisites

- Task 01 completed.
- Task 02 completed.
- Task 03 completed.
- Task 19 completed (KMS-compatible leaf-bundle provider/config exists).

## Blocked Preflight Notes

- As of 2026-07-07, this task has a planning/runtime contradiction to resolve before implementation:
  `docs/planning/c-mitm-leaf-certificate-design.md` requires one generated leaf private key and certificate per
  tenant/deployment/SNI, with KMS AAD scoped by `tenant_id`, `deployment_id`, normalized SNI, and CA identity/version.
  The current MITM runtime in `cmd/control/main.go` generates/selects the leaf certificate from TLS
  `GetCertificate`, which runs during the TLS handshake before HTTP request decoding, API-key authentication, and
  tenant identity are available.
- A placeholder/global tenant in the cache key or KMS AAD would violate the resolved tenant-scoped private-key policy.
  A deployment/SNI-only cache would be implementable, but it changes task 01's policy and should be an explicit
  planning decision, not an implementation shortcut.
- Per-tenant and global unique-SNI limits have the same tenant-before-handshake issue: global limits can run at
  `GetCertificate` time, but per-tenant limits need either tenant identity before TLS handshake or a revised limit
  model.
- Redis distributed locking, TTL capping, encrypted bundle storage, CA/KMS rotation, and local singleflight are
  implementable once the tenant identity timing decision is resolved.

## Out of Scope

- Do not change the private-key storage policy chosen in task 01.
- Do not implement MITM request dispatch beyond certificate lookup/generation.

## Expected Files

- Create or modify: MITM certificate cache package.
- Create or modify: Redis key/lock helpers if needed.
- Test: certificate cache tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Generate per-SNI leaf certificates signed by the configured CA.
- [ ] Store public certificate bytes and private-key material according to task 01.
- [ ] Enforce TTL no longer than configured cert validity days.
- [ ] Implement local singleflight and Redis distributed locking.
- [ ] Enforce bounded generation concurrency plus per-tenant and global unique-SNI limits.
- [ ] Add tests for cache hits, cache misses, encrypted storage, rotation, flood limits, Redis lock loss, and TTL.
- [ ] Run focused certificate cache tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused MITM certificate cache tests.
- `make check`

## Acceptance Criteria

- Leaf generation is coalesced and bounded.
- Stored private keys, if any, follow the resolved encryption policy.
- Unique-SNI floods cannot saturate Control CPU unchecked.

## Handoff Notes

- Document key prefixes, TTLs, and concurrency limits.

## Stop Conditions

- Stop if task 01's storage policy is incomplete.
- Stop if tenant identity is still unavailable at leaf certificate generation time and the planning docs still require
  tenant-scoped leaf cache keys/KMS AAD.
- Stop if a deferral would have no owning task file.

# 20 - MITM Leaf Certificate Cache

Status: done

## Objective

Implement MITM leaf certificate generation, encrypted Redis cache storage, TTL capping, miss coalescing, and generation
flood controls behind the tenant-aware certificate hook created by task 04.

## Context (gap being closed)

The original P2 task 04 was too large: it had to both reshape MITM runtime around authenticated CONNECT and implement
the encrypted shared cache. The runtime prerequisite is now owned by
`docs/tasks/p2/04-mitm-authenticated-connect-bootstrap.md`; this task owns only the cache/storage/coalescing/flood-control
slice.

Current leaf generation is uncached and in-memory only: `cmd/control/main.go:420` generates a new RSA key,
`cmd/control/main.go:451` signs a new leaf, and `cmd/control/main.go:456` returns a `tls.Certificate` containing the
private key. Task 19 added the provider boundary and AAD type in `internal/control/mitm_leaf_bundle_crypto.go`, but no
production code writes encrypted MITM leaf bundles to Redis, coalesces same-SNI misses, or bounds unique-SNI generation.
Section 21 explicitly lists P2 MITM cert cache/locks as Redis runtime state.

## Required Planning Docs

- `docs/planning/c-mitm-leaf-certificate-design.md` (KMS-backed shared cache, storage, TTL, rotation, coalescing, and
  required tests, lines ~25-100)
- `docs/planning/17-mitm-design-p2.md` (leaf certificate storage and cache miss coalescing, lines ~34-61)
- `docs/planning/21-state-and-storage.md` (Redis TTL discipline and MITM cert cache/locks, lines ~61-83)
- `docs/planning/24-static-configuration.md` (`control.deployment_id`, MITM CA/KMS config, and `STRAW_` env naming,
  lines ~5-24 and ~288-299)
- `docs/planning/27-security-controls.md` (private-key and secret logging restrictions, lines ~119-125)
- `docs/planning/33-risks.md` (Control CPU saturation risk, lines ~8-15)

## Prerequisites

- Task 04 completed (MITM leaf lookup/generation has authenticated tenant identity before TLS certificate selection).
- Task 19 completed (KMS-compatible leaf-bundle provider/config exists).
- Task 03 completed (operator-provided MITM CA config exists).
- Task 01 completed (KMS-backed tenant/deployment/SNI private-key storage policy is resolved).

## Out of Scope

- Do not implement authenticated CONNECT bootstrap; task 04 owns it.
- Do not change the private-key storage policy chosen in task 01.
- Do not add disk or object-storage leaf caches unless a required planning doc has made that storage mandatory and
  configured; Redis is the shared cache for this task.
- Do not implement tenant-admin CA configure/rotate APIs; task 18 owns those.
- Do not implement MITM HTTP/2 ALPN; task 16 owns that.

## Expected Files

- Add or modify: `internal/control/mitm_leaf_cache.go` for leaf generation, encrypted bundle serialization, cache keys,
  TTLs, local singleflight, generation bounds, and flood controls.
- Modify: `internal/control/mitm_leaf_bundle_crypto.go` only if the cache needs a small serialization helper.
- Modify: `internal/config/config.go` and `internal/config/config_test.go` for `control.deployment_id` /
  `STRAW_CONTROL_DEPLOYMENT_ID` loading and validation.
- Modify: `cmd/control/main.go` to wire the real cache into the tenant-aware MITM leaf hook from task 04.
- Test: `internal/control/mitm_leaf_cache_test.go`, `internal/config/config_test.go`, and focused `cmd/control` wiring
  tests.

## Steps

- [x] Read all required planning docs.
- [x] Load and validate the stable deployment ID used in cache keys and KMS AAD.
- [x] Define Redis key prefixes and AAD using tenant ID, deployment ID, normalized SNI, and CA identity/version.
- [x] Generate per-SNI leaf certificates signed by the configured CA with validity capped by
      `mitm_cert_validity_days`.
- [x] Store only encrypted serialized leaf bundles in Redis, with TTL no longer than the certificate's remaining
      validity.
- [x] Implement cache hit decrypt/reuse without regenerating the keypair.
- [x] Implement local singleflight and Redis distributed locking for same tenant/deployment/SNI misses, including lock
      TTL/loss behavior.
- [x] Enforce bounded generation concurrency and per-tenant plus global unique-SNI limits before generating a keypair.
- [x] Wire `cmd/control` to use the cache through task 04's tenant-aware hook; do not add any direct TLS/SNI-only cache
      path.
- [x] Add tests for cache miss, cache hit, encrypted storage, TTL cap, CA/KMS rotation behavior, local singleflight,
      Redis lock coalescing/loss, generation bounds, and per-tenant/global flood limits.
- [x] Run focused MITM leaf cache tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./internal/config ./cmd/control -run 'TestMITMLeaf|TestMITM|TestLoadControl'`
- `make check`

## Acceptance Criteria

- Cache miss generation stores an encrypted Redis bundle and serves the generated leaf.
- Cache hit decrypts the stored bundle and does not regenerate a keypair, proven by a focused test.
- Stored private keys are not present in plaintext Redis values, logs, ClickHouse, audit rows, or public errors.
- Cache TTL is capped by certificate remaining validity and configured `mitm_cert_validity_days`.
- CA identity/version and KMS key rotation behavior are tested: old leaves are invalidated or remain decryptable only
  during the documented overlap.
- Local singleflight and Redis distributed lock coalesce same tenant/deployment/SNI misses.
- Bounded generation concurrency and per-tenant/global unique-SNI flood limits reject excess unique names before
  keypair generation.
- No cache read/write path can run from a direct TLS `GetCertificate` callback with only SNI and no authenticated
  tenant identity.

## Handoff Notes

- Document Redis key prefixes, value shape, TTLs, lock TTLs, concurrency limits, and unique-SNI limit settings.
- Document the AAD fields and CA identity/version source used for encrypted bundles.
- Document Redis outage/fail policy and lock-loss behavior.
- Document Postgres-backed and live verification status.

## Stop Conditions

- Stop if task 04 has not made authenticated tenant identity available before leaf lookup/generation.
- Stop if task 19's provider boundary cannot encrypt/decrypt leaf bundles without adding a cloud/vendor SDK dependency.
- Stop if satisfying the required tests would require implementing optional disk or object-storage cache behavior not
  otherwise required by planning docs.
- Stop if a deferral would have no owning task file.

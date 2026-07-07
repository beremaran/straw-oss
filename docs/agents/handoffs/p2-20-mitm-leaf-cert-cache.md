# Handoff

Task: `docs/tasks/p2/20-mitm-leaf-cert-cache.md`

## Changed

- Added `internal/control/mitm_leaf_cache.go`: Redis-backed encrypted MITM leaf bundle cache with tenant/deployment/SNI/CA-scoped keys, KMS AAD, TTL capping, pre-expiry refresh, local singleflight, Redis locks, lock-loss recovery, process concurrency limits, and per-tenant/global unique-SNI rate controls.
- Added `internal/control/mitm_leaf_bundle_aws_kms.go`: stdlib AWS KMS envelope provider for `aws-kms`, using `GenerateDataKey`/`Decrypt`, SigV4, and local AES-256-GCM over the stored bundle.
- Wired `cmd/control` MITM startup to load the configured CA, build the real cache with Redis/KMS, and pass tenant-aware lookup plus preflight hooks into the authenticated CONNECT bootstrap.
- Added `control.deployment_id` / `STRAW_CONTROL_DEPLOYMENT_ID` loading and MITM validation, and documented the KMS/deployment config in `docs/planning/24-static-configuration.md`.
- Updated earlier task 04 and task 19 handoffs to mark their known leaf-cache gap closed by this task.

## Acceptance Criteria Verdicts

Independent verifier `019f3b56-af21-72f2-af3b-617e51c9fe92` returned VERIFIED after the retry-bypass and pre-expiry-refresh fixes.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Cache miss generation stores an encrypted Redis bundle and serves the generated leaf. | VERIFIED | `internal/control/mitm_leaf_cache.go:462`, `internal/control/mitm_leaf_cache.go:468`, `internal/control/mitm_leaf_cache.go:490` | `TestMITMLeafCacheMissStoresEncryptedBundleAndHitReusesIt`, `internal/control/mitm_leaf_cache_test.go:30` |
| Cache hit decrypts the stored bundle and does not regenerate a keypair. | VERIFIED | `internal/control/mitm_leaf_cache.go:408`, `internal/control/mitm_leaf_cache.go:430`, `internal/control/mitm_leaf_cache.go:437` | `TestMITMLeafCacheMissStoresEncryptedBundleAndHitReusesIt`, `internal/control/mitm_leaf_cache_test.go:72` |
| Stored private keys are not present in plaintext Redis values, logs, ClickHouse, audit rows, or public errors. | VERIFIED | Plain bundle is encrypted before Redis storage at `internal/control/mitm_leaf_cache.go:462`; cache code has no ClickHouse/audit/log write path. | Plaintext marker check at `internal/control/mitm_leaf_cache_test.go:56` |
| Cache TTL is capped by certificate remaining validity and configured `mitm_cert_validity_days`. | VERIFIED | `cmd/control/main.go:413`, `cmd/control/main.go:523`, `internal/control/mitm_leaf_cache.go:498` | `internal/control/mitm_leaf_cache_test.go:64` |
| CA identity/version and KMS key rotation behavior are tested. | VERIFIED | `cmd/control/main.go:547`, `internal/control/mitm_leaf_cache.go:569`, `internal/control/mitm_leaf_cache.go:641` | `TestMITMLeafCacheCAAndKMSRotationBehavior`, `internal/control/mitm_leaf_cache_test.go:84` |
| Local singleflight and Redis distributed lock coalesce same tenant/deployment/SNI misses. | VERIFIED | `internal/control/mitm_leaf_cache.go:336`, `internal/control/mitm_leaf_cache.go:508`, `internal/control/mitm_leaf_cache.go:535` | `TestMITMLeafCacheLocalSingleflightCoalescesMiss`, `TestMITMLeafCacheRedisLockCoalescesCrossInstanceMiss`, `internal/control/mitm_leaf_cache_test.go:176`, `internal/control/mitm_leaf_cache_test.go:211` |
| Bounded generation concurrency and per-tenant/global unique-SNI flood limits reject excess unique names before keypair generation. | VERIFIED | `internal/control/mitm_leaf_cache.go:301`, `internal/control/mitm_leaf_cache.go:306`, `internal/control/mitm_leaf_cache.go:595`, `internal/control/mitm_leaf_cache.go:618` | `TestMITMLeafCacheGenerationFloodLimitsRejectBeforeSecondGeneration`, `TestMITMLeafCacheUniqueSNIRateLimitsRejectSequentialFloodBeforeGeneration`, `internal/control/mitm_leaf_cache_test.go:292`, `internal/control/mitm_leaf_cache_test.go:358` |
| No cache read/write path can run from a direct TLS `GetCertificate` callback with only SNI and no authenticated tenant identity. | VERIFIED | CONNECT auth precedes preflight/tunnel at `internal/control/mitm_connect_handler.go:45`; TLS callback passes captured identity at `internal/control/mitm_connect_handler.go:96`; startup wires cache hooks via CONNECT at `cmd/control/main.go:375`. | `TestMITMConnectAuthenticatesBeforeLeafLookupAndServesInnerRequest`, `TestBuildMITMLeafLookupRequiresTenantIdentity` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `control.deployment_id` / `STRAW_CONTROL_DEPLOYMENT_ID` is required for encrypted MITM leaf storage. | implemented | `internal/config/config.go:59`, `internal/config/config.go:381`, `internal/config/config.go:393`, `docs/planning/24-static-configuration.md:125` |
| KMS-backed shared cache stores one leaf bundle per tenant/deployment/SNI and only Control can decrypt/use it. | implemented | `internal/control/mitm_leaf_cache.go:49`, `internal/control/mitm_leaf_cache.go:569`, `cmd/control/main.go:416` |
| Redis stores encrypted serialized bundles with tenant/deployment/SNI scope and TTLs. | implemented | Prefixes at `internal/control/mitm_leaf_cache.go:22`, value shape at `internal/control/mitm_leaf_cache.go:107`, TTL write at `internal/control/mitm_leaf_cache.go:490` |
| Plaintext private keys are not written to Redis, disk, object storage, logs, ClickHouse, audit rows, or public errors. | implemented | Plaintext is only passed to `EncryptMITMLeafBundle` before Redis `Set`: `internal/control/mitm_leaf_cache.go:462`; no disk/object/log/ClickHouse/audit writes in cache path. |
| Cache TTL never exceeds certificate remaining validity and is capped by configured validity. | implemented | `internal/control/mitm_leaf_cache.go:498`, `cmd/control/main.go:413`, `cmd/control/main.go:523` |
| Cached bundles refresh before expiry when remaining validity is inside the rotation window. | implemented | Default/explicit `RefreshWindow`: `internal/control/mitm_leaf_cache.go:59`, `internal/control/mitm_leaf_cache.go:196`; stale detection at `internal/control/mitm_leaf_cache.go:444`; test at `internal/control/mitm_leaf_cache_test.go:140` |
| CA rotation invalidates old generated leaves unless overlap is explicit. | implemented | CA identity/version are in AAD and key suffix: `cmd/control/main.go:547`, `internal/control/mitm_leaf_cache.go:569`, `internal/control/mitm_leaf_cache.go:641`; test changes CA version at `internal/control/mitm_leaf_cache_test.go:125` |
| KMS key rotation preserves decryptability during overlap and removes old-key dependence afterward. | implemented | Envelope decrypt path at `internal/control/mitm_leaf_cache.go:430`; overlap/old-key-disabled test at `internal/control/mitm_leaf_cache_test.go:107`, `internal/control/mitm_leaf_cache_test.go:116` |
| Local singleflight and Redis distributed locks coalesce same-SNI misses. | implemented | `internal/control/mitm_leaf_cache.go:336`, `internal/control/mitm_leaf_cache.go:508`; tests at `internal/control/mitm_leaf_cache_test.go:176`, `internal/control/mitm_leaf_cache_test.go:211` |
| Redis lock has short TTL and lock loss does not block generation indefinitely. | implemented | Default lock TTL at `internal/control/mitm_leaf_cache.go:27`, peer wait at `internal/control/mitm_leaf_cache.go:535`; lock-loss test at `internal/control/mitm_leaf_cache_test.go:264` |
| Bounded generation concurrency protects Control CPU. | implemented | Semaphore and active unique-SNI sets at `internal/control/mitm_leaf_cache.go:88`, `internal/control/mitm_leaf_cache.go:359`; test at `internal/control/mitm_leaf_cache_test.go:292` |
| Per-tenant/global unique-SNI floods fail with canonical overload/rate-limit error without generating a cert. | implemented | Cache limiter at `internal/control/mitm_leaf_cache.go:595`; CONNECT preflight at `internal/control/mitm_connect_handler.go:59`; canonical mapping at `internal/control/mitm_connect_handler.go:71`; test at `internal/control/mitm_handler_test.go:272` |
| Direct TLS/SNI-only `GetCertificate` path must not use placeholder/SNI-derived tenant. | implemented | Cache requires tenant identity at `internal/control/mitm_leaf_cache.go:235`; `cmd/control` wires only tenant-aware lookup/preflight at `cmd/control/main.go:432`; SNI callback uses captured authenticated identity at `internal/control/mitm_connect_handler.go:108`. |
| Static configuration docs list deployment ID and KMS env names. | implemented | `docs/planning/24-static-configuration.md:125`, `docs/planning/24-static-configuration.md:137`, `docs/planning/24-static-configuration.md:295` |

## Verification

```sh
go test ./internal/control -run 'TestMITMLeafCacheRefreshesNearExpiry|TestMITMLeafCacheUniqueSNIRateLimitsRejectSequentialFloodBeforeGeneration|TestMITMLeafCachePreflightRejectsUncachedUniqueSNIBeforeGeneration|TestMITMConnectLeafPreflightWritesRateLimitBeforeTunnel' -count=1
golangci-lint run --max-issues-per-linter 0 --max-same-issues 0
make check
STRAW_TEST_POSTGRES_DSN='postgres://postgres:postgres@localhost:5432/straw_test?sslmode=disable' go test ./...
```

Result: all passed.

- Postgres-backed tests: ran against `straw_test`.
- Live compose verification: skipped because the local compose config does not include task-20 MITM CA plus production `aws-kms` credentials. CONNECT/TLS bootstrap and cache behavior were exercised with handler-level integration tests and Redis-backed cache tests.

## Reviewer Start Points

- `internal/control/mitm_leaf_cache.go`
- `internal/control/mitm_leaf_cache_test.go`
- `internal/control/mitm_connect_handler.go`
- `cmd/control/main.go`
- `internal/config/config.go`

## Remaining Work

- None.

## Blockers

- None.

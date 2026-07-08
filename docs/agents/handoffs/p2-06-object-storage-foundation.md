# Handoff

Task: `docs/tasks/p2/06-object-storage-foundation.md`

## Changed

- `internal/objectstore/objectstore.go` (new) — the object-storage foundation. A `Client` presigns scoped,
  single-object S3 operations using stdlib SigV4 query signing (no new dependency):
  - `New(Options)` resolves credentials from the named env vars (never inline) and validates endpoint/bucket/region/retention.
  - `ObjectKey(tenantID, requestID, dir)` → `tenant/<tenant_id>/request/<request_id>/<direction>/<nonce>` with a
    128-bit `crypto/rand` nonce; rejects identifiers containing `/` or empty (tenant-prefix-escape guard).
  - `PresignGet` / `PresignPut` → short-lived SigV4 URLs bound to one key + one method; expiry clamped
    (default 5m, max 15m). `PresignPut` signs the `x-amz-server-side-encryption: AES256` header so uploads cannot omit SSE.
  - `Retention()` exposes the configured retention (1–3 days).
  - `ErrUnavailable`/`Unavailable`/`IsUnavailable` — the outage sentinel BodyRef/capture flows wrap transport failures
    with; Control maps it to `body_ref_unavailable` (Section 29 outage row).
  - No bucket-listing operation is exposed anywhere.
- `internal/objectstore/objectstore_test.go` (new) — key shape/entropy/collision, prefix-escape rejection, `New`
  validation matrix, presign scoping + short expiry, SSE enforcement, expiry clamp, outage sentinel, and a SigV4 core
  check pinned against AWS's published GET-Object signature vector.
- `internal/config/config.go` — added `errObjectStorageIncomplete` and `BodyObjectStorageConfig.complete()`, and a
  `validate()` branch so a Control config with `object_storage.enabled=true` but missing endpoint/bucket/region/cred-envs
  fails fast at load.
- `internal/config/config_test.go` — added the enabled-but-incomplete object-storage case.
- `docs/planning/30-testing-matrix.md` — extended the P2 BodyRef paragraph with the object-storage foundation test row.

## Acceptance Criteria Verdicts

From the independent verifier (general-purpose sub-agent, verify-straw-task method) — all VERIFIED:

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Object keys unguessable and tenant/request scoped | VERIFIED | `internal/objectstore/objectstore.go` `ObjectKey` (+ `safeIdentifier`) | `TestObjectKeyShapeAndEntropy` |
| Executors cannot list buckets | VERIFIED | only `PresignGet`/`PresignPut`/`ObjectKey`/`Retention` exported; no list op | N/A (absence) |
| Retention defaults to 1 day, capped at 3 | VERIFIED | `objectstore.go` Options validation + `config.go` `validate()` | `TestNewValidation`, `config_test.go` retention cases |
| Scoped short-lived signed URLs | VERIFIED | `presign` + `clampExpiry` | `TestPresignGetIsScopedAndShortLived` |
| SSE required on uploads | VERIFIED | `PresignPut` signs SSE header | `TestPresignPutEnforcesSSE` |
| Outage behavior defined | VERIFIED | `ErrUnavailable`/`IsUnavailable` | `TestUnavailableSentinel` |
| SigV4 core correct | VERIFIED | `awsSigningKey`/`hmacSHA256Hex` | `TestSigV4CoreMatchesAWSVector` (AWS vector) |

## Planning-Doc Coverage

| Planning item (Sections 18/24/29) | Status | Evidence / owning task |
|-----------------------------------|--------|------------------------|
| Object key `tenant/<id>/request/<id>/<direction>/<nonce>` + high-entropy nonce | implemented | `objectstore.go` `ObjectKey` |
| Unguessable keys, tenant-scoped by prefix | implemented | `ObjectKey` + `safeIdentifier` |
| SSE required where available | implemented | `PresignPut` signed SSE header |
| Signed URLs / temp creds expire quickly | implemented | `clampExpiry` (max 15m) |
| Executors cannot list buckets | implemented | no list op exposed |
| Retention default 1 day / max 3 days | implemented | Options + config validation |
| Object storage unavailable outage behavior (Section 29) | implemented | `ErrUnavailable` sentinel + mapping contract |
| Object storage config keys (Section 24) | already existed | `internal/config/config.go` `BodyObjectStorageConfig` (this task added completeness validation) |
| Request/response upload/download flows, checksum/size verify, multipart abort, lifecycle cleanup | out of scope | `docs/tasks/p2/07-...`, `08-...` |
| Payload-capture body-object writes | out of scope | `docs/tasks/p2/11-payload-capture-storage.md` |

## Verification

```sh
make check
```

Result: `0 issues`, all Go tests pass (`go test ./internal/objectstore/ ./internal/config/` green).

- Postgres-backed tests: not exercised — diff touches no `postgres_*` files or `migrations/`.
- Live compose verification: skipped — task 06 is a library foundation with no runtime request-path behavior yet;
  runtime construction/use is owned by tasks 07/08/11.

## Reviewer Start Points

- `internal/objectstore/objectstore.go`
- `internal/objectstore/objectstore_test.go`
- `internal/config/config.go` (`validate` / `complete`)

## Remaining Work

- None for task 06's scope. Runtime construction of `objectstore.Client` at Control startup and its request-path uses
  were completed by `docs/tasks/p2/07-bodyref-request-body-flow.md` (request upload),
  `docs/tasks/p2/08-bodyref-response-body-flow.md` (response tee), and `docs/tasks/p2/11-payload-capture-storage.md`
  (capture bodies). No fakes/stubs/TODOs introduced (grep clean); SigV4 signing is real stdlib crypto validated
  against an AWS vector.

## Blockers

- None.

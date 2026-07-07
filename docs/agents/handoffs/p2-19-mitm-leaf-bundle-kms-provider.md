# Handoff

Task: `docs/tasks/p2/19-mitm-leaf-bundle-kms-provider.md`

## Changed

- Added canonical `control.server.mitm_leaf_kms_provider` and `control.server.mitm_leaf_kms_key_id` config keys plus `STRAW_MITM_LEAF_KMS_PROVIDER` and `STRAW_MITM_LEAF_KMS_KEY_ID` env overrides.
- Added the MITM leaf-bundle crypto boundary in `internal/control`: provider interface, envelope shape, AAD helper, and provider/key config validation.
- Wired `cmd/control` startup to validate the configured provider/key boundary without creating a plaintext/static provider or enabling cache storage.
- Added tests for paired config validation, env overrides, unsafe provider rejection, fake-provider round trip, all AAD mismatch fields, rotation overlap, and ciphertext not containing PEM/private-key bytes.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `control.server` has documented, validated KMS provider/key config and matching `STRAW_` env vars. | VERIFIED | `docs/planning/24-static-configuration.md:22`, `internal/config/config.go:105`, `internal/config/config.go:436`, `internal/config/config.go:585`, `cmd/control/main.go:97` | `internal/config/config_test.go:97`, `internal/config/config_test.go:354`, `cmd/control/main_test.go:116` |
| Provider interface/envelope can encrypt/decrypt serialized leaf bundle without exposing plaintext private-key bytes. | VERIFIED | `internal/control/mitm_leaf_bundle_crypto.go:20`, `internal/control/mitm_leaf_bundle_crypto.go:34` | `internal/control/mitm_leaf_bundle_crypto_test.go:25`, `internal/control/mitm_leaf_bundle_crypto_test.go:36` |
| Decrypt rejects when tenant ID, deployment ID, SNI, or CA identity/version AAD does not match. | VERIFIED | `internal/control/mitm_leaf_bundle_crypto.go:45`, `internal/control/mitm_leaf_bundle_crypto_test.go:164` | `internal/control/mitm_leaf_bundle_crypto_test.go:48` |
| Rotation overlap is testable: old key decrypts during overlap, disabling it fails. | VERIFIED | `internal/control/mitm_leaf_bundle_crypto.go:37` | `internal/control/mitm_leaf_bundle_crypto_test.go:71` |
| No production code path provides plaintext/static deployment-key provider. | VERIFIED | `internal/control/mitm_leaf_bundle_crypto.go:20`, `internal/control/mitm_leaf_bundle_crypto.go:67`, `internal/config/config.go:590`; fake provider is `_test.go` only at `internal/control/mitm_leaf_bundle_crypto_test.go:115` | `internal/control/mitm_leaf_bundle_crypto_test.go:96` |
| Task 04 lists this task as a prerequisite. | VERIFIED | `docs/tasks/p2/04-mitm-leaf-cert-cache.md:20` | doc evidence |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Stored generated MITM leaf bundles must include public chain and private key, encrypted before leaving Control memory. | implemented | `internal/control/mitm_leaf_bundle_crypto.go:20`, `internal/control/mitm_leaf_bundle_crypto.go:34`; storage/cache write remains owned by `docs/tasks/p2/04-mitm-leaf-cert-cache.md`. |
| KMS-compatible shared cache policy with tenant/deployment scope. | implemented | `internal/control/mitm_leaf_bundle_crypto.go:45`; cache/Redis behavior remains owned by `docs/tasks/p2/04-mitm-leaf-cert-cache.md`. |
| KMS key rotation overlap must be testable. | implemented | `internal/control/mitm_leaf_bundle_crypto.go:37`, `internal/control/mitm_leaf_bundle_crypto_test.go:71`. |
| Canonical config keys under `control.server`. | implemented | `docs/planning/24-static-configuration.md:22`, `docs/planning/24-static-configuration.md:23`, `internal/config/config.go:105`, `internal/config/config.go:106`. |
| Canonical `STRAW_` env names. | implemented | `docs/planning/24-static-configuration.md:292`, `docs/planning/24-static-configuration.md:293`, `internal/config/config.go:436`, `internal/config/config.go:440`. |
| Private keys and secrets must not be written to logs, audit events, ClickHouse, or error responses. | implemented | This task writes no private key storage/logging path; ciphertext-only test at `internal/control/mitm_leaf_bundle_crypto_test.go:36`. |
| Resolved private-key policy is KMS-backed shared cache; rejected plaintext/static deployment-key options stay rejected. | implemented | `internal/control/mitm_leaf_bundle_crypto.go:67`, `internal/config/config.go:590`, `internal/control/mitm_leaf_bundle_crypto_test.go:96`. |

## Verification

```sh
go test ./internal/config ./internal/control ./cmd/control
make check
```

Result:

- Focused provider/config tests: passed.
- `make check`: passed; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reported `0 issues`.
- Postgres-backed tests: not exercised; diff does not touch Postgres files, migrations, or Postgres-backed behavior.
- Live compose verification: skipped; this task adds config validation and a provider/envelope boundary only, not runtime request path behavior or leaf-bundle storage.

## Reviewer Start Points

- `internal/control/mitm_leaf_bundle_crypto.go`
- `internal/control/mitm_leaf_bundle_crypto_test.go`
- `internal/config/config.go`
- `cmd/control/main.go`

## Remaining Work

- None for this task. Leaf-bundle cache/storage, Redis locks, local singleflight, TTLs, and flood controls remain owned by `docs/tasks/p2/04-mitm-leaf-cert-cache.md`.

## Blockers

- None.

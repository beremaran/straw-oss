# 19 - MITM Leaf Bundle KMS Provider

Status: done

## Objective

Add the minimal KMS-compatible encryption provider boundary and runtime config that task 20 can use before any
generated MITM leaf bundle containing private-key material is written outside Control memory.

## Context (gap being closed)

The original task 04 preflight found that it could not implement encrypted shared leaf-bundle storage without first
adding a KMS provider/config owner. The cache work is now task 20; it must store generated leaf certificate bundles
according to task 01 without changing the
resolved private-key policy. Appendix C requires generated bundles to include the public certificate chain and private
key, and requires stored bundles to be encrypted through a KMS-compatible mechanism before they leave Control memory.
Current Control config only exposes MITM enablement, port, CA files, and cert validity (`internal/config/config.go:97`),
with only a cert-validity env override (`internal/config/config.go:422`). The current MITM TLS path calls
`generateMITMLeaf` directly from `GetCertificate` (`cmd/control/main.go:332`), and `generateMITMLeaf` returns a
`tls.Certificate` containing the private key without any encrypted bundle/envelope mechanism
(`cmd/control/main.go:438`). This task owns the missing provider/config prerequisite so task 20 can stay focused on
cache, locks, TTL, and flood controls.

## Required Planning Docs

- `docs/planning/c-mitm-leaf-certificate-design.md` (resolved KMS-backed shared cache, storage, rotation, and tests,
  lines ~15-86)
- `docs/planning/17-mitm-design-p2.md` (leaf certificate storage policy, lines ~29-44)
- `docs/planning/24-static-configuration.md` (canonical config keys and `STRAW_` env naming, lines ~5-21 and ~284-292)
- `docs/planning/27-security-controls.md` (private-key and secret logging rules, lines ~119-125)
- `docs/planning/32-open-decisions.md` (P2 MITM private-key storage decision, lines ~38-46)

## Prerequisites

- Task 01 completed (resolved KMS-backed private-key storage policy).
- Task 03 completed (MITM CA/static config baseline exists).

## Out of Scope

- Do not implement the MITM leaf certificate cache, Redis keys/locks, local singleflight, or flood controls; task 20
  owns those.
- Do not add a cloud-provider SDK or new dependency.
- Do not add a plaintext, static deployment-key, or "dev convenience" provider that production code can use for stored
  leaf private keys.
- Do not change the private-key storage policy chosen in task 01.

## Expected Files

- Modify: `docs/planning/24-static-configuration.md` to name the canonical MITM leaf-bundle KMS config keys/env vars.
- Modify: `internal/config/config.go` and `internal/config/config_test.go` to load and validate the KMS provider/key
  config.
- Add: `internal/control/mitm_leaf_bundle_crypto.go` for the provider interface, encrypted envelope shape, and
  tenant/deployment/SNI additional-authenticated-data helper.
- Add: `internal/control/mitm_leaf_bundle_crypto_test.go` for provider/envelope tests using a test-only fake provider.
- Modify: `cmd/control/main.go` only as needed for `cmd/control` to construct/validate the configured provider without
  enabling cache storage before task 20.

## Steps

- [x] Read all required planning docs.
- [x] Add canonical config keys and env vars for the MITM leaf-bundle KMS provider name and key ID.
- [x] Validate that provider name and key ID are supplied together when MITM leaf bundle storage is configured.
- [x] Define a small provider interface for encrypt/decrypt and an envelope that records provider name, key ID/version,
      nonce/metadata needed by the provider, and ciphertext.
- [x] Define the AAD inputs as tenant ID, deployment ID, normalized SNI, and CA identity/version so ciphertext cannot be
      replayed across scopes.
- [x] Ensure production code has no plaintext/static-key provider; tests may use a fake provider in `_test.go`.
- [x] Wire `cmd/control` to construct or validate the configured provider boundary, but do not write any leaf bundle
      storage until task 20.
- [x] Add tests for config validation, encrypted envelope round trip through the fake provider, AAD mismatch rejection,
      key-version rotation overlap, and stored ciphertext not containing PEM/private-key bytes.
- [x] Run focused provider/config tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/config ./internal/control ./cmd/control`
- `make check`

## Acceptance Criteria

- `control.server` has documented, validated KMS provider/key config and matching `STRAW_` env vars.
- The provider interface and envelope can encrypt/decrypt a serialized leaf bundle in tests without exposing plaintext
  private-key bytes in the stored envelope.
- Decrypt rejects a bundle when tenant ID, deployment ID, SNI, or CA identity/version AAD does not match.
- Rotation overlap is testable: an old key version can decrypt during overlap, and disabling it causes decrypt failure.
- No production code path provides a plaintext or static deployment-key provider for stored leaf private keys.
- Task 20 lists this task as a prerequisite.

## Handoff Notes

- Document the config key names, env vars, envelope fields, AAD fields, fake-provider test behavior, and the exact
  constructor task 20 should call.

## Stop Conditions

- Stop if satisfying this task would require choosing a cloud/vendor KMS API or adding a dependency; ask instead.
- Stop if the config names would conflict with `docs/planning/24-static-configuration.md`.
- Stop if a deferral would have no owning task file.

# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T013`

## Changed

- Added a red executable-registry contract requiring the sole advertised name to map semantically to `profiles.Chrome_120` and explicitly excluding baseline aliases and unplanned browser presets.
- Added a semantic immutable-fixture comparison covering the normalized Chrome 120 TLS ClientHello, HTTP/2, protocol, and request-scoped transport contract.
- Added the intentionally empty JSON fixture that T014 must replace with the committed normalized contract before the tests can pass.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| The executable registry contains exactly `chrome_120 -> profiles.Chrome_120`. | VERIFIED (expected red contract) | `internal/egress/profile_registry_test.go:14` | `TestExecutableFingerprintProfileRegistryIsExact` fails because T014's registry does not exist yet. |
| Baseline/default and unplanned Firefox/Safari presets are never executable registry entries. | VERIFIED (expected red contract) | `internal/egress/profile_registry_test.go:36` | `TestExecutableFingerprintProfileRegistryIsExact` |
| Any semantic change to the committed normalized TLS/HTTP2 fixture is detected. | VERIFIED (expected red contract) | `internal/egress/profile_registry_test.go:51`, `internal/egress/profile_registry_test.go:133` | `TestChrome120GoldenFixtureIsImmutable` compares the parsed fixture with the complete pinned contract. |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Only `chrome_120` is planned and it maps to the pinned v1.15.1 `profiles.Chrome_120` preset. | test contract added | `internal/egress/profile_registry_test.go:14`; implementation owned by `specs/002-straw-fingerprint-profiles/tasks.md#T014` |
| Baseline and `default` remain outside the advertised named-profile registry. | test contract added | `internal/egress/profile_registry_test.go:36` |
| The finite golden covers normalized TLS and HTTP/2 characteristics while excluding incidental values. | test contract added | `internal/egress/profile_registry_test.go:133`; fixture population owned by `specs/002-straw-fingerprint-profiles/tasks.md#T014`; observer owned by T022 |

## Verification

```sh
cd straw && go test ./internal/egress
cd straw && go test ./internal/egress -run 'Test(ExecutableFingerprintProfileRegistryIsExact|Chrome120GoldenFixtureIsImmutable)$'
cd straw && git diff --check
```

Result:

- The existing Egress package passed before T013's red tests were added.
- The focused T013 command now fails only at the intended missing `executableFingerprintProfiles` compile-time registry. After T014 adds that registry, the intentionally empty fixture remains red until T014 commits the full normalized contract.
- `git diff --check` passes.
- Postgres-backed tests were not exercised because this diff does not touch Postgres surfaces.
- Live compose verification is not applicable to this test-first registry slice; T025 and T042-T043 own the integrated and live gates.

## Reviewer Start Points

- `internal/egress/profile_registry_test.go:14`
- `internal/egress/profile_registry_test.go:51`
- `internal/egress/testdata/chrome_120_v1_15_1.json`

## Remaining Work

- T014 owns the compile-time registry and complete normalized fixture required to turn these tests green.
- T022 owns the independent local observer that compares live normalized observations with this fixture and baseline.

## Blockers

- None. Changes are uncommitted.

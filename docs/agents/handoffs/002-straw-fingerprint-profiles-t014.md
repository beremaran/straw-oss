# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T014`

## Changed

- Added the exact compile-time Egress registry mapping `chrome_120` to `profiles.Chrome_120`.
- Replaced the placeholder fixture with the committed normalized Chrome 120 v1.15.1 TLS/HTTP2 contract.
- Corrected the two T013 fixture-test error assignments required by the repository's `noinlineerr` lint rule.

## Acceptance Criteria Verdicts

Fresh read-only verifier verdict: **VERIFIED** on 2026-07-10.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Registry contains only exact `chrome_120 -> profiles.Chrome_120` | VERIFIED | `straw/internal/egress/profile_registry.go:5` | `TestExecutableFingerprintProfileRegistryIsExact` |
| Baseline, aliases, and unplanned presets are excluded | VERIFIED | `straw/internal/egress/profile_registry.go:5` | `TestExecutableFingerprintProfileRegistryIsExact` |
| Normalized Chrome 120 v1.15.1 TLS/HTTP2 contract is committed and immutable | VERIFIED | `straw/internal/egress/testdata/chrome_120_v1_15_1.json:1` | `TestChrome120GoldenFixtureIsImmutable` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| One initial named preset, exact `profiles.Chrome_120` mapping | implemented | `straw/internal/egress/profile_registry.go:5` |
| Finite normalized TLS and HTTP/2 golden fixture | implemented | `straw/internal/egress/testdata/chrome_120_v1_15_1.json:1` |
| Local observed conformance comparison | out of scope | `specs/002-straw-fingerprint-profiles/tasks.md#T022` |

## Verification

```sh
cd straw
go test ./internal/egress -run 'Test(ExecutableFingerprintProfileRegistryIsExact|Chrome120GoldenFixtureIsImmutable)' -count=1
go test ./internal/egress -count=1
make check
```

Result: all commands passed; `golangci-lint` reported `0 issues`.

- Postgres-backed tests: not exercised; the diff does not touch Postgres surfaces.
- Live compose verification: not exercised; T014 is the compile-time registry/fixture slice and live conformance is owned by T022/T043.

## Reviewer Start Points

- `straw/internal/egress/profile_registry.go`
- `straw/internal/egress/testdata/chrome_120_v1_15_1.json`
- `straw/internal/egress/profile_registry_test.go`

## Remaining Work

- None for T014.

## Blockers

- None.

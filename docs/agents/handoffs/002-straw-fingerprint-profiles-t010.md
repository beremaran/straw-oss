# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T010`

## Resolution Update (2026-07-10)

The T014 registry and later story/live entries below are historical snapshots. T014 closed the registry source,
T025/T034/T039 closed story acceptance, and T042-T043 closed full/live verification. Final evidence:
`002-straw-fingerprint-profiles-complete.md` and
`specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Changed

- Added the SDK protocol 1.1 capability field and immutable registration copy.
- Made the official Egress worker advertise the exact `chrome_120` capability.
- Added the optional worker-credential allowlist field, including the existing Postgres JSON persistence path.
- Copied exact capability lists into replacement-safe Control sessions, tenant-filtered candidate/views, and Redis current/superseded records.
- Applied the minimum lint cleanup to the T009 red tests once T010 made them compile, and added focused tenant-view and official-worker assertions.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| SDK advertises an immutable capability list under protocol 1.1 | VERIFIED | `sdk/egress/types.go:25`, `sdk/egress/types.go:125` | `TestBuildRegisterRequestCopiesFingerprintProfiles` |
| Official Egress registration advertises exact `chrome_120` support | VERIFIED | `internal/egress/registration.go:27`, `cmd/egress/main.go:217` | `TestBuildRegisterRequestAdvertisesFingerprintProfiles`, `TestBuildCapabilitiesFromConfig` |
| Credential allowlists preserve empty-as-unrestricted subset semantics | VERIFIED | `internal/control/worker_credential_store.go:36`, `internal/control/worker_registry.go:614` | `TestRegisterFingerprintCapabilitySubset` |
| Replacement sessions own immutable current capabilities and stale sessions cannot restore old claims | VERIFIED | `internal/control/worker_registry.go:358` | `TestRegisterReplacementUsesOnlyNewFingerprintCapabilities`, `TestStaleSessionHeartbeatCannotRestoreFingerprintCapabilities` |
| Tenant-filtered candidates/views expose copied capabilities without session internals | VERIFIED | `internal/control/worker_registry.go:502`, `internal/control/worker_registry.go:933` | `TestTenantViewsCopyFingerprintCapabilitiesWithoutSessionInternals` |
| Redis round-trips independent current and superseded capability lists | VERIFIED | `internal/control/worker_runtime_redis.go:107`, `internal/control/worker_runtime_redis.go:150` | `TestRedisWorkerRuntimeRoundTripsImmutableFingerprintCapabilities` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Signed SDK and official-worker capability propagation | implemented | `sdk/egress/types.go:125`; `cmd/egress/main.go:217` |
| Optional credential capability allowlist and Postgres JSON round trip | implemented | `internal/control/worker_credential_store.go:36`; `internal/control/postgres_worker_credential_store.go:26`; `internal/control/postgres_worker_credential_store.go:212` |
| Immutable replacement-session and tenant-safe runtime projections | implemented | `internal/control/worker_registry.go:358`; `internal/control/worker_registry.go:524`; `internal/control/worker_registry.go:936` |
| Redis current/superseded capability persistence | implemented | `internal/control/worker_runtime_redis.go:107`; `internal/control/worker_runtime_redis.go:150`; `internal/control/worker_runtime_redis.go:203` |
| Compile-time executable preset registry backing the advertised list | owned follow-on | `specs/002-straw-fingerprint-profiles/tasks.md#T014` |

## Verification

```sh
cd straw && go test ./sdk/egress ./internal/egress ./internal/control ./cmd/egress
cd straw && make check
make check-straw
```

Result:

- Focused tests: PASS.
- Straw-local full tests and lint: PASS (`0 issues`).
- Root `make check-straw`: PASS.
- Postgres-backed tests: not exercised because `STRAW_TEST_POSTGRES_DSN` is unset; the capability uses the existing generic `allowed_capabilities_jsonb` marshal/unmarshal path and requires no schema change.
- Live compose verification: not applicable to this foundational registration/runtime-state task; story acceptance remains owned by T025, T034, T039, and T042-T043.

## Reviewer Start Points

- `sdk/egress/types.go:76`
- `cmd/egress/main.go:206`
- `internal/control/worker_registry.go:156`
- `internal/control/worker_runtime_redis.go:107`

## Remaining Work at Handoff (Historical; Resolved)

- T014 replaces the bounded official capability source with the compile-time executable preset registry before named execution is enabled.

## Blockers

- None. Changes are intentionally uncommitted.

# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T009`

## Resolution Update (2026-07-10)

The failing-test and T010 production-work entries below are historical snapshots. T010 closed every listed production
gap; final verification is recorded in `002-straw-fingerprint-profiles-complete.md` and
`specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Changed

- Added the SDK registration-copying red test in `sdk/egress/types_test.go`.
- Added the official Egress registration-advertisement red test in `internal/egress/registration_test.go`.
- Added credential allowlist, session replacement, and stale-session isolation red tests in `internal/control/worker_registry_test.go`.
- Added immutable current/superseded capability Redis round-trip coverage in `internal/control/worker_runtime_redis_test.go`.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| SDK copies the immutable capability list and uses the capability-aware protocol minor | VERIFIED RED | `sdk/egress/types_test.go:61` | `TestBuildRegisterRequestCopiesFingerprintProfiles` |
| Official Egress registration advertises the exact list without retaining caller-owned storage | VERIFIED RED | `internal/egress/registration_test.go:9` | `TestBuildRegisterRequestAdvertisesFingerprintProfiles` |
| Credential profile allowlists preserve empty-as-unrestricted subset semantics | VERIFIED RED | `internal/control/worker_registry_test.go:265` | `TestRegisterFingerprintCapabilitySubset` |
| Replacement registration atomically installs only the new immutable list | VERIFIED RED | `internal/control/worker_registry_test.go:369` | `TestRegisterReplacementUsesOnlyNewFingerprintCapabilities` |
| A superseded-session heartbeat cannot restore its prior capabilities | VERIFIED RED | `internal/control/worker_registry_test.go:396` | `TestStaleSessionHeartbeatCannotRestoreFingerprintCapabilities` |
| Redis round-trips current/superseded lists without sharing mutable slices | VERIFIED RED | `internal/control/worker_runtime_redis_test.go:9` | `TestRedisWorkerRuntimeRoundTripsImmutableFingerprintCapabilities` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Signed SDK/Egress capability propagation | failing tests added | `sdk/egress/types_test.go:61`; `internal/egress/registration_test.go:9`; production is owned by T010 |
| Credential subset validation | failing tests added | `internal/control/worker_registry_test.go:265`; production is owned by T010 |
| Immutable replacement and stale-session isolation | failing tests added | `internal/control/worker_registry_test.go:369`; production is owned by T010 |
| Redis runtime capability persistence | failing tests added | `internal/control/worker_runtime_redis_test.go:9`; production is owned by T010 |

## Verification

```sh
cd straw && go test ./sdk/egress ./internal/egress ./internal/control
```

Result:

- Before T009 edits: PASS for all three packages.
- After T009 edits: expected FAIL because `Capabilities.SupportedFingerprintProfiles`, `WorkerCapabilities.SupportedFingerprintProfiles`, and `runtimeSession.supportedFingerprintProfiles` do not exist yet. These are the exact T010 production seams; no unrelated failure was reported.
- `gofmt` and `git diff --check`: PASS.
- Postgres-backed tests: not exercised; T009 does not touch Postgres behavior.
- Live compose verification: not applicable to this test-first foundational task.

## Reviewer Start Points

- `sdk/egress/types_test.go:61`
- `internal/egress/registration_test.go:9`
- `internal/control/worker_registry_test.go:265`
- `internal/control/worker_runtime_redis_test.go:9`

## Remaining Work at Handoff (Historical; Resolved)

- T010 owns the production propagation required to make these tests green.

## Blockers

- None. Changes are intentionally uncommitted.

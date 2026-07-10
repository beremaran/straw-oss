# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`  
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T030-T034`

## Changed

- Implemented duplicate-member, non-string, malformed-UTF-8, exact-value, and literal-`baseline` request validation in `internal/control/request.go`; REST and stream ingress share this validator, while proxy ingress remains baseline-only.
- Kept unsafe requested-profile evidence bounded through the existing hash-plus-byte-length projection in `internal/control/request_metadata.go`.
- Moved named-profile support validation out of durable worker flags and into post-ordinary routing capability filtering in `internal/control/destination_policy.go` and `internal/control/routing.go`; zero-capacity sessions remain ineligible.
- Added deterministic profile availability classification and tenant-safe worker/pool reconciliation to `internal/control/config_admin_handlers.go`.
- Preserved fail-closed Egress behavior by checking the local executable registry before resolver/client construction in `internal/egress/profile_registry.go`, `internal/egress/executor.go`, and `internal/egress/profiled_transport.go`.
- Propagated selected/executed profile fields into buffered rejection evidence in `internal/control/dispatcher.go`; Control-local rejection keeps both fields empty, and Egress drift keeps executed empty.
- Updated the Redis sticky-router test fixture to provide explicit available capacity required by the routing eligibility contract.

## Acceptance Criteria Verdicts

Fresh verifier: the focused US2 matrix and `make check-straw` both pass on the current worktree.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Duplicate members, non-string/malformed UTF-8 values, literal `baseline`, and exact profile intent are rejected or preserved before dispatch | VERIFIED | `internal/control/request.go:134`, `internal/control/request.go:197`, `internal/control/request.go:266` | `TestValidateRequestRejectsDuplicateFingerprintProfileMembers`, `TestValidateRequestRejectsLiteralBaselineFingerprintProfile`, `TestValidateRequestRejectsMalformedUTF8FingerprintProfile` |
| Unsafe requested values are projected as bounded hash-plus-byte-length evidence | VERIFIED | `internal/control/request_metadata.go:229`, `internal/control/request_metadata.go:283` | `TestProjectFingerprintEvidenceClassifiesValues` |
| Ordinary tenant/route/session eligibility and capacity remain authoritative before exact capability filtering | VERIFIED | `internal/control/routing.go:141`, `internal/control/routing.go:188`, `internal/control/routing.go:325`; `internal/control/destination_policy.go:423` | `TestResolveDestinationPolicyEnabledNamedProfileDoesNotDependOnWorkerAvailability`, `TestDispatcherNamedFingerprintPreservesOrdinaryRouteUnavailable`, `TestFingerprintProfileTunnelPreparationPreservesRouteNoMatch` |
| Diagnostics distinguish supported, disabled, missing executable definition, and no active capable worker without worker/session internals | VERIFIED | `internal/control/config_admin_handlers.go:1131`, `internal/control/config_admin_handlers.go:1202` | `TestFingerprintProfilesReadOnlySeparatesSupportedAndUnavailableReasons` |
| Unsupported Egress instructions are rejected before DNS/client construction and emit no `OutboundStart` | VERIFIED | `internal/egress/profile_registry.go:15`, `internal/egress/executor.go:409`, `internal/egress/profiled_transport.go:35` | `TestUnsupportedFingerprintUnknownInstructionNoUpstream`, `TestUnsupportedFingerprintCapabilityDriftNoUpstream` |
| Control-local and Egress-drift rejection evidence is one correlated, redacted, non-retryable outcome with empty executed profile | VERIFIED | `internal/control/dispatcher.go:256`, `internal/control/request_metadata.go:241` | `TestHandlerUnsupportedFingerprintRecordsSingleCorrelatedEvent`, `TestApplyRequestOutcomeUnsupportedFingerprintKeepsExecutedProfileEmpty`, `TestFingerprintProfileBaselineAllowsEmptyExecutedValue` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Request-side duplicate/exact validation and baseline rejection | implemented | `internal/control/request.go:197`; T030 |
| Safe requested-profile evidence projection | implemented and reused | `internal/control/request_metadata.go:283`; T030/T033 |
| Ordinary eligibility before capability filtering | implemented | `internal/control/routing.go:149`; T031 |
| Tenant-safe availability diagnostics | implemented | `internal/control/config_admin_handlers.go:1202`; T031 |
| Zero-upstream unsupported execution proof | implemented and verified | `internal/egress/profile_registry.go:15`; T032 |
| Correlated Control-local/Egress-drift rejection evidence | implemented | `internal/control/dispatcher.go:256`; T033 |
| US2 focused verification and handoff | complete | this handoff; T034 |

## Verification

```sh
go test ./internal/control -run 'Test(ValidateRequestRejectsDuplicateFingerprintProfileMembers|ValidateRequestRejectsLiteralBaselineFingerprintProfile|ValidateRequestRejectsMalformedUTF8FingerprintProfile|ProjectFingerprintEvidenceClassifiesValues|ApplyRequestOutcomeUnsupportedFingerprintKeepsExecutedProfileEmpty|ResolveDestinationPolicyEnabledNamedProfileDoesNotDependOnWorkerAvailability|DispatcherNamedFingerprintPreservesOrdinaryRouteUnavailable|FingerprintProfileTunnelPreparationPreservesRouteNoMatch|FingerprintProfilesReadOnlySeparatesSupportedAndUnavailableReasons|HandlerUnsupportedFingerprintRecordsSingleCorrelatedEvent|FingerprintProfileBaselineAllowsEmptyExecutedValue)' -count=1
go test ./internal/egress -run 'TestUnsupportedFingerprint(UnknownInstruction|CapabilityDrift)NoUpstream' -count=1
go test ./internal/control -run 'Test(FingerprintProfile|WorkerFingerprintCapability|Dispatcher.*Fingerprint)' -count=1
go test ./internal/egress -run 'Test.*UnsupportedFingerprint.*NoUpstream' -count=1
go test ./internal/egress -run 'Test(ProfileRegistry|ProfileConformance|ProfilePinnedDial|ProfileConnectionIsolation|ProfileDeadline|ProfileCancellation|ProfileErrorMapping)' -count=1
go test ./internal/control ./internal/egress -run 'Test.*(Baseline|DefaultFingerprint|Unprofiled)' -count=1
make check-straw
```

Result:

- Every focused US2, no-upstream, diagnostics, redaction, conformance, and baseline command passed.
- `make check-straw` passed: all Straw tests and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reported clean.
- Postgres-backed tests were not exercised; no Postgres behavior changed in this slice.
- Live compose/Coles verification was skipped; it belongs to the later unified acceptance gate and is not required for US2.

## Reviewer Start Points

- `internal/control/request.go:134`
- `internal/control/routing.go:141`
- `internal/control/config_admin_handlers.go:1131`
- `internal/control/dispatcher.go:256`
- `internal/egress/profile_registry.go:15`
- `internal/egress/profiled_transport.go:35`
- `internal/control/request_metadata.go:283`

## Remaining Work

- T035-T039 remain as the separately owned US3 baseline/regression slice.
- T040-T046 remain as later documentation, governance, full-stack, live acceptance, analysis, and completion tasks.
- No US2 gap is deferred without an owning task.

## Blockers

- None. Changes are uncommitted in the shared worktree.

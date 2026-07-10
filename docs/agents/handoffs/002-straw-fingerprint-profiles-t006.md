# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T006`

## Changed

- Added reflection-based protobuf contract tests for the additive `RegisterRequest.supported_fingerprint_profiles = 19` and `OutboundStartFrame.executed_fingerprint_profile = 6` fields, including legacy registration-byte preservation and wire round trips.
- Added registration-signing tests that preserve legacy-minor bytes, require new-minor capability binding, canonicalize profile order, and reject a capability mutation.
- Marked T006 complete after the tests demonstrated the intended pre-T007 failure.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test / check |
|-----------|---------|----------------------------|----------------------|
| T007 must add the two additive wire fields with their assigned numbers and preserve legacy registration encoding. | VERIFIED (expected pre-implementation failure) | `api/proto/straw/v1/contract_test.go:87` | `TestFingerprintProfileFieldsAreAdditiveAndRoundTrip` fails because field 19 is absent. |
| T008 must preserve legacy signing bytes for old workers, bind capabilities for the new minor, canonicalize ordering, and invalidate a changed capability list. | VERIFIED (expected pre-implementation failure) | `api/proto/straw/v1/registration_sign_test.go:13`; `api/proto/straw/v1/registration_sign_test.go:35` | The tests fail at the absent field; after T007 they intentionally remain red until T008 implements signing. |
| Existing protobuf behavior remains executable before the new tests are selected. | VERIFIED | `api/proto/straw/v1/contract_test.go:14` | Focused pre-existing contract test selection passed. |

## Verification

```sh
cd straw && go test ./api/proto/straw/v1
cd straw && go test ./api/proto/straw/v1 -run 'Test(FingerprintProfileFieldsAreAdditiveAndRoundTrip|RegistrationSigningPayloadPreservesLegacyBytesUntilNewMinorCapabilities|RegistrationSignatureCanonicalizesProfileOrderAndRejectsMutation)$'
cd straw && go test ./api/proto/straw/v1 -run 'Test(StreamFrameBodyRefCompiles|AssignRequestCreditFieldsExist|ExecutorDelegatedDestinationResolutionKeepsWireNumber|ValidateRejectsUnknownEnums)$'
```

- Baseline `go test ./api/proto/straw/v1` passed before adding T006 tests.
- The three new focused tests fail as intended because `RegisterRequest.supported_fingerprint_profiles` does not yet exist. This is the red baseline required by T006; T007 owns the generated-wire change and T008 owns the signing implementation.
- The focused pre-existing contract test selection passed after the changes.
- Live verification: not applicable to this test-only protocol task; T025, T034, T039, and T042 own the feature's integration/live checks.

## Reviewer Start Points

- `api/proto/straw/v1/contract_test.go:87`
- `api/proto/straw/v1/registration_sign_test.go:13`
- `api/proto/straw/v1/registration_sign_test.go:35`

## Remaining Work

- T007 — add fields 19 and 6, increment the official worker protocol minor, regenerate bindings, and synchronize the canonical contract.
- T008 — implement legacy-byte-preserving and new-minor capability signing/verification.

## Blockers

- None.

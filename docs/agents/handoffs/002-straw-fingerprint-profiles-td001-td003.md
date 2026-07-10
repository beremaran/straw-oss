# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#TD001-TD003`

## Changed

- Decoupled omitted and explicit `default` requests from the disabled durable `default` row while preserving baseline transport selection.
- Returned canonical `unsupported_fingerprint` for literal `baseline` and malformed UTF-8 profile values, with one safely projected rejection event before REST or streaming dispatch.
- Rejected every non-canonical worker fingerprint capability claim before session creation while preserving empty legacy-minor baseline registration.
- Replaced the enabled-default fixture with the migrated/in-memory disabled state and added focused regression coverage.

## Acceptance Criteria Verdicts

The post-change SpecKit consistency pass found full requirement coverage and no constitution conflict or unowned gap.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Omitted and explicit `default` select baseline independently of the disabled durable row | VERIFIED | `straw/internal/control/destination_policy.go:431` | `TestResolveDestinationPolicy_AllowsOrdinaryPublicHost`, `TestResolveDestinationPolicy_DefaultAliasUsesBaselineWithDisabledDurableRow`, `TestDispatcherBaselineAndDefaultUseEmptyWireInstruction` |
| Literal `baseline` and malformed UTF-8 return canonical unsupported evidence before assignment | VERIFIED | `straw/internal/control/request.go:266`, `straw/internal/control/handler.go:128`, `straw/internal/control/stream_handler.go:76` | `TestValidateRequestRejectsLiteralBaselineFingerprintProfile`, `TestValidateRequestRejectsMalformedUTF8FingerprintProfile`, `TestUnsupportedFingerprintValidationRecordsOneEventWithoutDispatch` |
| Registration rejects non-canonical claims before creating a session and preserves legacy baseline compatibility | VERIFIED | `straw/internal/control/worker_registry.go:632` | `TestRegisterRejectsNonCanonicalFingerprintCapabilitiesBeforeSessionCreation`, `TestRegisterPreservesLegacyBaselineAndAcceptsCanonicalProfileClaim`, `TestRegisterFingerprintCapabilitySubset` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Profile catalog baseline alias | implemented | `straw/internal/control/destination_policy.go:431`; TD001 |
| Operational rejection evidence and SC-004/SC-008 | implemented | `straw/internal/control/handler.go:128`; TD002 |
| Canonical signed capability claims and legacy-minor compatibility | implemented | `straw/internal/control/worker_registry.go:632`; TD003 |

## Verification

```sh
cd straw && go test ./internal/control ./internal/egress -run 'Test.*(Baseline|DefaultFingerprint|Unprofiled)'
cd straw && go test ./internal/control -run 'Test(ValidateRequestRejectsLiteralBaselineFingerprintProfile|ValidateRequestRejectsMalformedUTF8FingerprintProfile|UnsupportedFingerprintValidationRecordsOneEventWithoutDispatch|HandlerUnsupportedFingerprintRecordsSingleCorrelatedEvent|FingerprintProfile|Dispatcher.*Fingerprint)'
cd straw && go test ./internal/egress -run 'Test.*UnsupportedFingerprint.*NoUpstream'
cd straw && go test ./internal/control -run 'TestRegister(FingerprintCapabilitySubset|RejectsNonCanonicalFingerprintCapabilitiesBeforeSessionCreation|PreservesLegacyBaselineAndAcceptsCanonicalProfileClaim|Valid)'
make check-straw
git diff --check
```

Result: all focused commands passed; `make check-straw` passed all Go packages and `golangci-lint` with 0 issues at base revision `ba53199` plus the uncommitted slice.

- Postgres-backed tests: not exercised; the diff does not touch Postgres surfaces and uses the already-verified migrated/in-memory disabled-row state.
- Live compose verification: not required for TD001-TD003; the bounded focused and adjacent gates passed.

## Reviewer Start Points

- `straw/internal/control/destination_policy.go:431`
- `straw/internal/control/handler.go:128`
- `straw/internal/control/request.go:266`
- `straw/internal/control/worker_registry.go:632`

## Remaining Work

- None.

## Blockers

- None. Changes remain uncommitted for user review.

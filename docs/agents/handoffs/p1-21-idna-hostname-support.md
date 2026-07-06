# Handoff

Task: `docs/tasks/p1/21-idna-hostname-support.md`

## Changed

- Added `golang.org/x/net/idna` and normalized REST target URL hostnames to A-labels during Control URL validation.
- Reused the same hostname normalizer for destination-policy checks and admin-authored host/CNAME deny rules.
- Added IDNA dispatch, homograph-deny, and invalid-IDNA validation tests.
- Updated the stale P0 task 22 IDNA notes now that this gap is closed.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| A valid internationalized hostname dispatches successfully end-to-end. | VERIFIED | `internal/control/request.go:255`, `internal/control/request.go:289`, `internal/control/dispatcher_test.go:529` | `TestDispatcherControlNATSEgressRoundTripIDNAHost` |
| A homograph of a denied hostname is still denied. | VERIFIED | `internal/control/destination_policy.go:186`, `internal/control/config_admin_handlers.go:673` | `TestResolveDestinationPolicy_IDNAHostDenyNormalization` |
| Invalid IDNA input is rejected with the existing validation error code. | VERIFIED | `internal/control/request.go:263`, `internal/control/request.go:66` | `TestValidateRequestRejectsInvalidIDNAHost` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Section 27: deny-rule evaluation must normalize lowercase hostnames, IDNA/punycode, and trailing dots. | implemented | `internal/control/request.go:278`, `internal/control/config_admin_handlers.go:673`, `internal/control/destination_policy.go:186` |
| Section 27: policy evaluation must run before dispatch and deny Unicode look-alike bypasses. | implemented | `internal/control/destination_policy_test.go:84` |
| Section 27: invalid destinations must fail closed with destination/validation errors and no raw URL leakage. | implemented | `internal/control/request.go:263`, `internal/control/destination_policy_test.go:104` |
| Section 15: Control rejects client-supplied `Host`; Egress derives Host from the URL. | already existed | `internal/control/request.go:285`; normalized URL host is what Egress receives through existing dispatch flow. |

## Verification

```sh
go test ./internal/control -run 'Test(ResolveDestinationPolicy_IDNAHostDenyNormalization|ValidateRequestRejectsInvalidIDNAHost|DispatcherControlNATSEgressRoundTripIDNAHost|DenyRuleValidationAndRoleRestriction)' -count=1
go test ./internal/control -count=1
make check
```

Result:

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped because the task behavior is covered by the in-process live NATS + egress + upstream dispatch test and does not require docker-compose-only infrastructure.

## Reviewer Start Points

- `internal/control/request.go`
- `internal/control/destination_policy.go`
- `internal/control/config_admin_handlers.go`
- `internal/control/dispatcher_test.go`
- `internal/control/destination_policy_test.go`

## Remaining Work

- None.

## Blockers

- None.

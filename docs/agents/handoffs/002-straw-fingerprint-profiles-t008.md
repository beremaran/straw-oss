# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T008`

## Resolution Update (2026-07-10)

The T010 propagation and later integration/live entries below are historical snapshots. T010 closed propagation and
T025/T034/T039/T042-T043 closed integration/live verification; final evidence is in
`002-straw-fingerprint-profiles-complete.md` and `specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Changed

- Extended registration signing for protocol minor 1 and later with a counted, byte-length-prefixed capability list.
- Canonicalized capabilities on a cloned slice by sorting and deduplicating, so signing does not mutate the request.
- Preserved the exact legacy signing payload for older protocol minors and empty capability lists.
- Applied the user-authorized mechanical lint cleanup to the adjacent T006 protobuf tests.
- Marked only T008 complete.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test / check |
|-----------|---------|----------------------------|----------------------|
| Older protocol minors retain byte-for-byte legacy signing payloads even if capabilities are present. | VERIFIED | `api/proto/straw/v1/registration_sign.go:56` | `TestRegistrationSigningPayloadPreservesLegacyBytesUntilNewMinorCapabilities` |
| Empty capability lists retain legacy signing bytes. | VERIFIED | `api/proto/straw/v1/registration_sign.go:57` | Full protobuf package and Straw gates |
| New-minor capability signing is deterministic across input order and binds profile mutations. | VERIFIED | `api/proto/straw/v1/registration_sign.go:58` | `TestRegistrationSignatureCanonicalizesProfileOrderAndRejectsMutation` |
| Capability entries are sorted, unique, counted, and byte-length-prefixed without mutating the protobuf slice. | VERIFIED | `api/proto/straw/v1/registration_sign.go:58` | Focused signing tests plus code inspection; Control validation remains owned by T010 |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Protocol 1.1 signs a counted, length-prefixed, sorted unique capability list. | implemented | `api/proto/straw/v1/registration_sign.go:56` |
| Older minors and empty lists retain exact legacy signing bytes. | implemented | `api/proto/straw/v1/registration_sign.go:57` |
| Signing and verification bind the same canonical payload. | already existed and extended by the shared payload function | `api/proto/straw/v1/registration_sign.go:78`; `api/proto/straw/v1/registration_sign.go:84` |
| Registration validation and runtime propagation reject invalid claims and store immutable capabilities. | out of T008 scope | T010 |

## Verification

```sh
cd straw && go test ./api/proto/straw/v1 -run 'TestRegistration(SigningPayloadPreservesLegacyBytesUntilNewMinorCapabilities|SignatureCanonicalizesProfileOrderAndRejectsMutation)$'
cd straw && go test ./api/proto/straw/v1
cd straw && golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./api/proto/straw/v1/...
cd straw && make check
git diff --check
```

Result: all final commands passed. An initial full-gate run exposed a transient
`TestMITMHTTP2StreamCancelIsIsolated` failure and six adjacent T006 lint findings. The isolated test passed on rerun;
the user authorized the mechanical test lint cleanup, and the complete final `make check` passed with zero lint issues.

- Postgres-backed tests: not exercised; this diff does not touch Postgres surfaces.
- Live compose verification: not exercised; T008 is limited to deterministic registration signing, with live and
  integration verification owned by T025, T034, T039, and T042.

## Reviewer Start Points

- `api/proto/straw/v1/registration_sign.go:56`
- `api/proto/straw/v1/registration_sign_test.go:17`
- `api/proto/straw/v1/registration_sign_test.go:39`

## Remaining Work at Handoff (Historical; Resolved)

- T010 propagates protocol 1.1 capabilities through worker registration, validation, sessions, and Redis state.

## Blockers

- None. Changes are uncommitted.

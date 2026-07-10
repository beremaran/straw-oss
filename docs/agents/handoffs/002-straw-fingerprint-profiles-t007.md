# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T007`

## Changed

- Added `RegisterRequest.supported_fingerprint_profiles` at field 19 and
  `OutboundStartFrame.executed_fingerprint_profile` at field 6.
- Regenerated the Go binding with the repository-pinned `protoc-gen-go` v1.36.10.
- Declared official worker protocol 1.1 and synchronized capability, compatibility, and executed-profile semantics in
  the canonical protobuf planning contract.
- Marked only T007 complete.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test / check |
|-----------|---------|----------------------------|----------------------|
| Register capability field is additive at field 19 and preserves legacy registration bytes. | VERIFIED | `api/proto/straw/v1/straw.proto:167`; `api/proto/straw/v1/straw.pb.go:978` | `TestFingerprintProfileFieldsAreAdditiveAndRoundTrip` |
| Outbound executed-profile evidence is additive at field 6 and round-trips. | VERIFIED | `api/proto/straw/v1/straw.proto:344`; `api/proto/straw/v1/straw.pb.go:2829` | `TestFingerprintProfileFieldsAreAdditiveAndRoundTrip` |
| Official worker protocol minor and compatibility rules are canonicalized as 1.1. | VERIFIED | `docs/planning/13-protobuf-contract.md:13`; `docs/planning/13-protobuf-contract.md:243` | Manual contract comparison against `contracts/wire-protocol.md` |
| Generated descriptors match the source schema. | VERIFIED | `api/proto/straw/v1/straw.pb.go:3181`; `api/proto/straw/v1/straw.pb.go:3346` | `buf lint`; `buf build` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Additive registration capability field 19 | implemented | `api/proto/straw/v1/straw.proto:167` |
| Additive executed-profile field 6 | implemented | `api/proto/straw/v1/straw.proto:344` |
| Protocol 1.1 compatibility and canonical signing rules | documented; signing implementation remains planned | `docs/planning/13-protobuf-contract.md:13`; T008 |
| Official Egress capability advertisement and runtime propagation | out of T007 scope | T010 |

## Verification

```sh
cd straw && PATH="<pinned-v1.36.10-tool-dir>:$PATH" buf generate
cd straw && buf lint
cd straw && buf build
cd straw && go test ./api/proto/straw/v1 -run 'Test(StreamFrameBodyRefCompiles|AssignRequestCreditFieldsExist|ExecutorDelegatedDestinationResolutionKeepsWireNumber|ValidateRejectsUnknownEnums|FingerprintProfileFieldsAreAdditiveAndRoundTrip)$'
make check-protos
cd straw && git diff --check
```

Result: all commands passed. The full protobuf package still has the intentionally red T008 mutation assertion
(`TestRegistrationSignatureCanonicalizesProfileOrderAndRejectsMutation`); T008 owns capability signing. Postgres-backed
tests and live compose verification were not exercised because T007 changes only the additive protobuf contract.

## Reviewer Start Points

- `api/proto/straw/v1/straw.proto:167`
- `api/proto/straw/v1/straw.proto:344`
- `docs/planning/13-protobuf-contract.md:243`

## Remaining Work

- T008 implements registration capability signing.
- T010 propagates protocol 1.1 and capabilities through the official worker/runtime path.

## Blockers

- None.

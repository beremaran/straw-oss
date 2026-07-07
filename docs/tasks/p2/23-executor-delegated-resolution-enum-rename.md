# 23 - Executor-Delegated Resolution Enum Rename

Status: done

## Objective

Rename the provider-adapter destination-resolution enum value to executor-delegated terminology while preserving wire
number 3, reserving the old name, regenerating protobuf code, and updating validation/docs so non-generated code no
longer uses Provider Adapter naming.

## Context (gap being closed)

The 2026-07-07 `P2 Provider Adapter Baseline` decision superseded the Provider Adapter concept, but the protobuf and
docs still expose `DESTINATION_RESOLUTION_PROVIDER_ADAPTER`. Evidence: `api/proto/straw/v1/straw.proto` defines
`DESTINATION_RESOLUTION_PROVIDER_ADAPTER = 3`; `api/proto/straw/v1/validate.go` accepts the generated constant; and
`docs/planning/27-security-controls.md` says the rename is owned by the P2 Egress SDK task. The original task 12 mixed
this compatibility-sensitive rename with the SDK extraction; this task owns it separately.

## Required Planning Docs

- `docs/planning/13-protobuf-contract.md` (compatibility rules; reserved names/numbers; Buf checks)
- `docs/planning/16-egress-execution.md` (Executor-delegated mode)
- `docs/planning/27-security-controls.md` (Executor-delegated resolution section)
- `docs/planning/32-open-decisions.md` (superseded `P2 Provider Adapter Baseline` entry)

## Prerequisites

- None. This rename is independent from the SDK code extraction, but must land before task 13 documents custom
  implementations publicly.

## Out of Scope

- Do not change wire number 3 or the `DestinationPolicy.resolution_mode` field number.
- Do not change destination-policy behavior.
- Do not implement custom Egress examples or SDK runtime behavior.

## Expected Files

- Modify: `api/proto/straw/v1/straw.proto` — rename enum value at wire number 3 and reserve the old name.
- Modify: generated protobuf files under `api/proto/straw/v1/`.
- Modify: `api/proto/straw/v1/validate.go`.
- Modify: `docs/planning/13-protobuf-contract.md` and `docs/planning/27-security-controls.md`.
- Test: protobuf contract/validation tests as needed.

## Steps

- [x] Read all required planning docs.
- [x] Rename `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` to executor-delegated naming while keeping value `3`.
- [x] Reserve the old enum value name per the protobuf contract.
- [x] Regenerate protobuf code.
- [x] Update validation and planning docs to use the new enum name and remove the "rename owned by" parenthetical.
- [x] Run Buf lint/breaking checks and focused protobuf tests.
- [x] Verify non-generated Provider Adapter references are either the reserved proto name or historical decision text.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `buf lint`
- `buf breaking --against '.git#branch=main'`
- `go test ./api/proto/straw/v1`
- `make check`

## Acceptance Criteria

- The enum value with wire number `3` is named for executor-delegated resolution in generated Go code.
- The old `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` name is reserved in `straw.proto`.
- `api/proto/straw/v1/validate.go` accepts the new generated enum constant.
- `docs/planning/27-security-controls.md` no longer says the rename is owned by the P2 Egress SDK task.
- Buf lint and Buf breaking checks pass; if Buf breaking rejects the rename despite reserving the old name, stop and
  ask instead of forcing it.

## Handoff Notes

- Record the new enum value name.
- Record Buf lint and breaking-check results.
- Record the grep used to prove stale Provider Adapter naming is gone from non-generated code except historical notes
  or the reserved proto name.

## Stop Conditions

- Stop if Buf breaking checks reject the enum rename even with the old name reserved.
- Stop if a deferral would have no owning task file.

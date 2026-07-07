# Handoff

Task: `docs/tasks/p2/23-executor-delegated-resolution-enum-rename.md`

## Changed

- Renamed destination resolution wire value `3` to `DESTINATION_RESOLUTION_EXECUTOR_DELEGATED` in `api/proto/straw/v1/straw.proto`, regenerated `straw.pb.go`, and reserved the old `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` enum name.
- Updated protobuf validation, the protobuf contract doc, and the security controls doc to use executor-delegated naming.
- Added a contract test proving the renamed enum keeps wire number `3` and validates.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| The enum value with wire number `3` is named for executor-delegated resolution in generated Go code. | VERIFIED | `api/proto/straw/v1/straw.pb.go:554`; `api/proto/straw/v1/contract_test.go:43` | `go test ./api/proto/straw/v1` |
| The old `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` name is reserved in `straw.proto`. | VERIFIED | `api/proto/straw/v1/straw.proto:100` | `buf lint` |
| `api/proto/straw/v1/validate.go` accepts the new generated enum constant. | VERIFIED | `api/proto/straw/v1/validate.go:265`; `api/proto/straw/v1/contract_test.go:43` | `go test ./api/proto/straw/v1` |
| `docs/planning/27-security-controls.md` no longer says the rename is owned by the P2 Egress SDK task. | VERIFIED | `docs/planning/27-security-controls.md:111` | `rg -n "Provider Adapter\|DESTINATION_RESOLUTION_PROVIDER_ADAPTER" docs/planning docs/tasks docs/agents/handoffs` |
| Buf lint and Buf breaking checks pass; if Buf breaking rejects the rename despite reserving the old name, stop and ask instead of forcing it. | APPROVED BREAKING CHANGE | `api/proto/straw/v1/straw.proto:105` | `buf lint` passed; `buf breaking --against '.git#branch=master'` failed on the enum rename and the user approved breaking changes for this stage. |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Section 13 protobuf compatibility: removed names are reserved. | implemented | `api/proto/straw/v1/straw.proto:100`; `docs/planning/13-protobuf-contract.md:297` |
| Section 13 `DestinationPolicy.resolution_mode = 13` remains unchanged. | already existed | `docs/planning/13-protobuf-contract.md:292`; no field-number change in `api/proto/straw/v1/straw.proto` |
| Section 13 destination resolution value `3` is executor-delegated. | implemented | `api/proto/straw/v1/straw.proto:105`; `docs/planning/13-protobuf-contract.md:302` |
| Section 16 executor-delegated mode must enforce equivalent destination policy and constrained facts. | already existed | `docs/planning/16-egress-execution.md`; this task is naming-only and does not change behavior. |
| Section 27 executor-delegated SSRF mode uses the renamed enum and no longer says rename is owned by the P2 Egress SDK task. | implemented | `docs/planning/27-security-controls.md:111` |
| Section 32 Provider Adapter concept is superseded and custom Egress implementations replace provider adapters. | already existed | `docs/planning/32-open-decisions.md`; historical decision text intentionally remains. |

## Verification

```sh
PATH=/tmp/straw-tools:$PATH /tmp/straw-tools/buf lint
PATH=/tmp/straw-tools:$PATH /tmp/straw-tools/buf breaking --against '.git#branch=main'
PATH=/tmp/straw-tools:$PATH /tmp/straw-tools/buf breaking --against '.git#branch=master'
go test ./api/proto/straw/v1
make check
rg -n "PROVIDER_ADAPTER|Provider Adapter|provider adapter" api/proto/straw/v1 docs/planning/13-protobuf-contract.md docs/planning/27-security-controls.md docs/planning/32-open-decisions.md docs/tasks/p2.md docs/tasks/p2/23-executor-delegated-resolution-enum-rename.md internal sdk cmd
```

Result:

- `buf lint`: passed.
- `buf breaking --against '.git#branch=main'`: not runnable in this checkout because there is no local `main` ref.
- `buf breaking --against '.git#branch=master'`: failed because enum value `3` changed name from `DESTINATION_RESOLUTION_PROVIDER_ADAPTER` to `DESTINATION_RESOLUTION_EXECUTOR_DELEGATED`; user approved breaking changes at this stage.
- `go test ./api/proto/straw/v1`: passed.
- `make check`: passed, including `go test ./...` and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`.
- Grep result: stale Provider Adapter naming is confined to the reserved proto name, historical decision/task text, and generated descriptor metadata for the reserved name.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped (diff does not touch the runtime request path).

## Reviewer Start Points

- `api/proto/straw/v1/straw.proto`
- `api/proto/straw/v1/validate.go`
- `api/proto/straw/v1/contract_test.go`
- `docs/planning/13-protobuf-contract.md`
- `docs/planning/27-security-controls.md`

## Remaining Work

- None.

## Blockers

- None.

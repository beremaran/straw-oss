# Handoff

Task: `docs/tasks/p2/05-bodyref-transport-selection.md`

## Changed

- `internal/control/body_transport.go` — added doc comments on exported types/consts/func; refactored
  `ValidateBodyRefFrame` to extract `s3RefUsable`/`directStreamRefUsable` helpers (cyclomatic complexity fix). The
  selection logic (`SelectBodyTransport`) and error mapping already existed and were verified against Section 18.
- `internal/config/config.go` — added a doc comment on the large-body transport const block; removed ineffective
  `omitempty` on nested struct JSON tags (`body_transport`, `object_storage`, `direct_stream`). Config keys,
  defaults, normalization, and validation (only `stream_through_control_tee_object_storage` accepted) already existed.
- `internal/control/dispatcher.go` — whitespace fix (`wsl_v5`) above the `SelectBodyTransport` call in
  `acceptResponseData`.
- `docs/planning/30-testing-matrix.md` — added the P2 BodyRef transport-selection test-row paragraph required before
  the feature ships (per `docs/tasks/p2.md` notes).

Net: the transport-selection code and tests were present from a prior commit but had never been made lint-clean and
the task/board status was never flipped; this run made `make check` green, added the missing testing-matrix row, and
completed the task bookkeeping.

## Acceptance Criteria Verdicts

From the independent verifier (general-purpose sub-agent, verify-straw-task method):

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Body transport selection matches the Section 18 table | VERIFIED | `internal/control/body_transport.go:39-67` | `TestSelectBodyTransportMatchesSection18Table` |
| Only the decided response-body mode is enabled | VERIFIED | `internal/config/config.go:494-496` | `internal/config/config_test.go` (unsupported-mode case) |
| P0 DataFrame behavior unchanged for small bodies | VERIFIED | `internal/control/dispatcher.go:1008-1015` | `TestSelectBodyTransportMatchesSection18Table` (small-body case) |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Section 18 selection table (DataFrames/S3/DirectStream/body_too_large) | implemented | `internal/control/body_transport.go:39-67` |
| Object-storage-before-direct-stream precedence | implemented | `body_transport.go:52-64` + test case |
| `body_too_large` with `direction`/`limit_bytes` details (Section 14) | implemented | `body_transport.go:92-100` |
| `body_ref_unavailable` for disabled/unusable variants (Section 14) | implemented | `ValidateBodyRefFrame` + helpers `body_transport.go:69-96` |
| Config: threshold + enabled transports + retention (Section 24) | implemented | `internal/config/config.go:138-162`, defaults `464-504` |
| Only resolved response-body mode validates (Section 32) | implemented | `config.go:494-496` |
| S3 request/response upload/tee runtime, checksum/size, retention enforcement, outage | out of scope | `docs/tasks/p2/06-...`, `07-...`, `08-...` |

## Verification

```sh
make check
```

Result: `0 issues`, all Go tests pass.

- Postgres-backed tests: not exercised — diff touches no `postgres_*` files or `migrations/`.
- Live compose verification: skipped — task 05 is selection logic/config only; large-body runtime flows (which would
  be observable live) are owned by tasks 06–08. Response selection is wired into `acceptResponseData` and covered by
  unit tests.

## Reviewer Start Points

- `internal/control/body_transport.go`
- `internal/config/config.go` (body-transport section)
- `internal/control/dispatcher.go:997-1017`

## Remaining Work

- None for task 05's scope. The "executor streams through Control while teeing to object storage" runtime was completed
  by `docs/tasks/p2/06-object-storage-foundation.md` (client) and
  `docs/tasks/p2/08-bodyref-response-body-flow.md` (tee flow).

## Blockers

- None.

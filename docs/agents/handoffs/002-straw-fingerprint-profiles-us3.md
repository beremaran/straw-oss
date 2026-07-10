# Handoff

Feature: `specs/002-straw-fingerprint-profiles/`
Task: `specs/002-straw-fingerprint-profiles/tasks.md#T035-T039`

## Resolution Update (2026-07-10)

The later T040-T046 polish/live entries below are historical snapshots of the bounded US3 run. T040-T046 closed all
listed feature-level work; final evidence is in `002-straw-fingerprint-profiles-complete.md` and
`specs/002-straw-fingerprint-profiles/evidence/live-coles.md`.

## Changed

- `straw/internal/control/destination_policy.go`: canonicalizes omitted/`default` requests to `baseline` after validating the compatibility catalog entry.
- `straw/internal/control/dispatcher.go`: keeps baseline off the wire as an empty fingerprint instruction.
- `straw/internal/control/dispatcher_test.go`: verifies omitted and explicit `default` dispatch through the baseline path and evidence.
- `straw/internal/egress/executor_test.go`: covers baseline origin-status passthrough and baseline/named pool-key isolation; existing profiled transport tests cover request-scoped cleanup, cancellation, deadlines, streaming, and no reuse.
- `specs/002-straw-fingerprint-profiles/tasks.md`: marks T035–T039 complete.

## Acceptance Criteria Verdicts

Fresh verifier: `go test` and `make check` on 2026-07-10.

| Criterion | Verdict | Implementation | Proving test |
|-----------|---------|----------------|--------------|
| Omitted and explicit `default` select/evidence `baseline` | VERIFIED | `straw/internal/control/destination_policy.go:434`, `straw/internal/control/lifecycle.go:90` | `TestDispatcherBaselineAndDefaultUseEmptyWireInstruction` |
| Baseline wire instruction remains empty | VERIFIED | `straw/internal/control/dispatcher.go:1781` | `TestDispatcherBaselineAndDefaultUseEmptyWireInstruction` |
| Baseline preserves origin status passthrough | VERIFIED | `straw/internal/egress/executor.go:443` | `TestExecutorBaselineRegressionMatrixPreservesOriginStatuses` |
| Baseline pool identity remains compatible; named traffic is isolated | VERIFIED | `straw/internal/egress/executor.go:768` | `TestExecutorBaselineUsesExistingRetryAndPoolBoundaries`, `TestExecutorUpstreamConnectionPoolKeyIncludesIsolationFields` |
| Named client cleanup, cancellation, deadlines, streaming, and no cross-request reuse remain bounded | VERIFIED | `straw/internal/egress/profiled_transport.go:40` | `TestProfileConnectionIsolationDoesNotFollowRedirects`, `TestProfileCancellationStopsPinnedDial`, `TestProfileDeadlineDuringPinnedDial`, `TestProfileConformanceStreamsBodyAndLateTrailers` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Baseline alias semantics and empty wire instruction | implemented | `straw/internal/control/destination_policy.go:434`, `straw/internal/control/dispatcher.go:1781` |
| Existing net/http baseline retry/pool path | already existed and preserved | `straw/internal/egress/executor.go:443`, `straw/internal/egress/executor.go:519` |
| Request-scoped named transport cleanup | already existed and verified | `straw/internal/egress/profiled_transport.go:40` |
| Baseline regression/isolation acceptance matrix | implemented | T035–T036 tests and this handoff |

## Verification

```sh
make check-protos
cd straw && go test ./internal/egress -run 'Test(ProfileRegistry|ProfileConformance|ProfilePinnedDial|ProfileConnectionIsolation|ProfileDeadline|ProfileCancellation|ProfileErrorMapping|ExecutorBaseline)' -count=1
cd straw && go test ./internal/control -run 'Test.*(Baseline|DefaultFingerprint|Unprofiled)' -count=1
cd straw && make check
```

Result:

- All focused tests passed.
- `cd straw && make check` passed: all Go tests and golangci-lint reported zero issues.
- Postgres-backed tests: not exercised; this slice does not modify Postgres surfaces.
- Live compose verification: not exercised; the US3 independent test is the local baseline/isolation matrix, while the feature-level live Coles gate remains owned by T043.

## Reviewer Start Points

- `straw/internal/control/destination_policy.go:428`
- `straw/internal/control/dispatcher.go:1778`
- `straw/internal/egress/profiled_transport.go:30`
- `straw/internal/egress/executor.go:393`
- `straw/internal/control/dispatcher_test.go:471`
- `straw/internal/egress/executor_test.go:85`

## Remaining Work at Handoff (Historical; Resolved)

- None within User Story 3. Feature-level polish/live work remains explicitly owned by T040–T046.

## Blockers

- None. Changes are uncommitted in the working tree.

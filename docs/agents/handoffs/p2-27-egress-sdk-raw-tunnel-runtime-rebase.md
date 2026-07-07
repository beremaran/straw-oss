# Handoff

Task: `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md`

## Changed

- Added the public `sdk/egress.TunnelOpener` and `TunnelTarget` seam for raw CONNECT tunnel opening.
- Moved raw tunnel stream runtime into `sdk/egress`: tunnel start routing, e2c outbound/response/data/end/error/cancel frames, c2e upload data, upload credit grants, download credit gating, and cancellation.
- Wired the official worker to the SDK tunnel seam through `internal/egress.tunnelAdapter`; destination validation and actual dial remain in `internal/egress.Executor.openTunnel`.
- Added SDK raw tunnel tests for data flow, upload credit, download credit, and cancellation.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `internal/egress` no longer owns raw tunnel stream protocol except through the SDK-facing official dial interface. | VERIFIED | `sdk/egress/assignment.go:361`; `internal/egress/registration.go:110`; `internal/egress/worker.go:15`; `internal/egress/executor.go:600` | `go test ./sdk/... ./internal/egress ./cmd/egress`; `make check` |
| SDK tests prove raw tunnel data flow, upload credit, and cancellation. | VERIFIED | `sdk/egress/assignment_test.go:302`; `sdk/egress/assignment_test.go:364` | `TestSDKRawTunnelRuntimeDataFlowAndUploadCredit`; `TestSDKRawTunnelRuntimeDownloadCreditAndCancellation`; `make check` |
| `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches. | VERIFIED | `sdk/egress/types.go:3`; `sdk/egress/assignment.go:3` | `rg '"github.com/beremaran/straw/v2/internal/' sdk/egress` returned no matches |
| Existing official executor tests still pass. | VERIFIED | `internal/egress/executor.go:600`; `internal/egress/executor.go:1115` | `go test ./sdk/... ./internal/egress ./cmd/egress`; `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| `docs/planning/05-component-boundaries.md`: SDK owns assignment handling, stream framing, and error protocol for custom Egress implementations. | implemented | Raw tunnel stream protocol lives in `sdk/egress/assignment.go:361`; public tunnel seam is `sdk/egress/types.go:55`. |
| `docs/planning/05-component-boundaries.md`: official Egress Worker remains the reference implementation built on the SDK. | implemented | Official worker wires SDK `TunnelOpener` in `internal/egress/registration.go:68` and `internal/egress/worker.go:15`. |
| `docs/planning/12-nats-protocol.md`: raw `c2e` carries upload `DataFrame`, `CancelFrame`, and download credit. | implemented | `sdk/egress/assignment.go:438`; `TestSDKRawTunnelRuntimeDataFlowAndUploadCredit`; `TestSDKRawTunnelRuntimeDownloadCreditAndCancellation`. |
| `docs/planning/12-nats-protocol.md`: raw `e2c` carries `OutboundStartFrame`, `ResponseStart`, tunnel download `DataFrame`, `EndFrame`, `ErrorFrame`, `CancelledFrame`, and upload credit. | implemented | `sdk/egress/assignment.go:428`; `sdk/egress/assignment.go:483`; `sdk/egress/assignment.go:535`; `sdk/egress/assignment.go:539`; `sdk/egress/assignment.go:543`; `sdk/egress/assignment.go:547`; `sdk/egress/assignment.go:551`; `sdk/egress/assignment.go:555`; `sdk/egress/assignment.go:559`. |
| `docs/planning/12-nats-protocol.md`: stream frames are sequenced, credit-governed, and cancellation-aware. | implemented | Shared validator/credit path is used by raw tunnel runtime in `sdk/egress/assignment.go:262`, `sdk/egress/assignment.go:438`, `sdk/egress/assignment.go:447`, and `sdk/egress/assignment.go:492`; tests at `sdk/egress/assignment_test.go:302` and `sdk/egress/assignment_test.go:364`. |
| `docs/planning/16-egress-execution.md`: official outbound execution, destination-policy validation, and dial target invariant stay in the worker. | implemented | Official tunnel adapter delegates to `internal/egress.Executor.openTunnel` in `internal/egress/registration.go:110`; validation/dial stay in `internal/egress/executor.go:600` and `internal/egress/executor.go:1115`. |
| BodyRef request-body runtime movement. | out of scope | Owned by `docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`. |
| Final SDK conformance/live verification. | out of scope | Owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md` after tasks 27, 31, and 28. |

## Verification

```sh
go test ./sdk/... ./internal/egress ./cmd/egress -count=1
golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./sdk/egress ./internal/egress ./cmd/egress
rg '"github.com/beremaran/straw/v2/internal/' sdk/egress
make check
```

Result:

- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because task 24 owns final SDK conformance and live compose verification after tasks 27, 31, and 28.
- Grep/audit terms: `fake` hits are existing/new test fakes only; no runtime fake, stub, TODO, or unowned deferral was introduced.

## Reviewer Start Points

- `sdk/egress/types.go`
- `sdk/egress/assignment.go`
- `sdk/egress/assignment_test.go`
- `internal/egress/registration.go`
- `internal/egress/worker.go`

## Remaining Work

- BodyRef request-body runtime rebase remains owned by `docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`.
- SDK runtime test migration and wiring proof remains owned by `docs/tasks/p2/28-egress-sdk-runtime-test-migration-and-wiring-proof.md`.
- Final SDK conformance and live compose verification remains owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`.

## Blockers

- None.

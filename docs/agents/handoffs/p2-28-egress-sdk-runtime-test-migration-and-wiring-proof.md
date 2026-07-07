# Handoff

Task: `docs/tasks/p2/28-egress-sdk-runtime-test-migration-and-wiring-proof.md`

## Changed

- `cmd/egress/main.go`: changed the command runtime seam to call `sdk/egress.Run` directly and construct the official SDK worker through `internal/egress.NewWorker`.
- `internal/egress/assignment_test.go`, `internal/egress/registration_test.go`: deleted stale protocol-runtime tests now covered by `sdk/egress`.
- `internal/egress/registration.go`: deleted the unused production `FakeExecutor` test helper.
- `internal/egress/executor_test.go`: kept official outbound execution coverage and moved its tenant constant local to that file.
- `docs/tasks/p2.md`, `docs/tasks/p2/28-egress-sdk-runtime-test-migration-and-wiring-proof.md`: marked task 28 done after independent verification.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Protocol-runtime coverage lives under `sdk/egress`; `internal/egress` tests cover official outbound execution and official-only adapters. | VERIFIED | `sdk/egress/types_test.go:19`, `sdk/egress/assignment_test.go:123`, `internal/egress/executor_test.go:32`, `internal/egress/registration.go:27` | `go test ./sdk/... ./internal/egress ./cmd/egress` |
| `cmd/egress` has a wiring proof that the built path uses the SDK runtime instead of `internal/egress.Run`. | VERIFIED | `cmd/egress/main.go:229`, `cmd/egress/main.go:273`, `cmd/egress/main_test.go:61` | `go test ./cmd/egress -run TestRunWorkerUsesSDKRuntime` |
| `sdk/egress` imports no `internal/*` packages. | VERIFIED | `sdk/egress/types.go:3`, `sdk/egress/runtime.go:3`, `sdk/egress/assignment.go:3`, `sdk/egress/bodyref.go:3` | `grep -R "\"github.com/beremaran/straw/v2/internal/" -n sdk/egress` returned no matches |
| Task 24 can proceed to independent SDK conformance and live compose verification. | VERIFIED | `cmd/egress/main.go:273`, `sdk/egress/runtime.go:30`, `sdk/egress/assignment_test.go:123`, `sdk/egress/runtime_test.go:82` | `go test ./sdk/... ./internal/egress ./cmd/egress`; `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Official Egress Worker is the reference implementation built on the public SDK. | implemented | `cmd/egress/main.go:273` calls `sdk/egress.Run`; `cmd/egress/main_test.go:61` proves the command path reaches the SDK runtime seam. |
| SDK owns NATS registration, heartbeat, assignment handling, stream framing, and error protocol. | already existed | `sdk/egress/runtime.go:30`, `sdk/egress/assignment.go:63`, `sdk/egress/types_test.go:19`, `sdk/egress/assignment_test.go:123`, `sdk/egress/runtime_test.go:82`. |
| `internal/egress` remains official outbound execution and official-only adapters. | implemented | Stale protocol tests were removed; official execution coverage remains in `internal/egress/executor_test.go:32`; official adapters remain in `internal/egress/registration.go:88`. |
| Registration protocol: register request/reply and heartbeat. | already existed | `sdk/egress/runtime.go:30`; `sdk/egress/types_test.go:19`; `sdk/egress/runtime_test.go:82`. |
| Assignment protocol: exact-session assignment, stream subjects, sequence/credit/error behavior. | already existed | `sdk/egress/assignment.go:63`; `sdk/egress/assignment_test.go:123`, `sdk/egress/assignment_test.go:154`, `sdk/egress/assignment_test.go:180`, `sdk/egress/assignment_test.go:201`, `sdk/egress/assignment_test.go:251`. |
| Egress SDK test rows before feature ships. | implemented | Runtime/protocol test ownership now sits under `sdk/egress`; task 24 owns the next independent SDK conformance and live compose verification step. |

## Verification

```sh
go test ./sdk/... ./internal/egress ./cmd/egress
grep -R "\"github.com/beremaran/straw/v2/internal/" -n sdk/egress || true
make check
```

Result:

- Focused tests: passed.
- SDK internal import grep: no matches.
- `make check`: passed, including `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`.
- Postgres-backed tests: not exercised; diff does not touch Postgres surfaces.
- Live compose verification: skipped; task 28 explicitly excludes live compose verification and assigns it to `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`.

## Reviewer Start Points

- `cmd/egress/main.go`
- `sdk/egress/assignment_test.go`
- `sdk/egress/runtime_test.go`
- `internal/egress/executor_test.go`

## Remaining Work

- None for task 28. Live SDK conformance and compose verification remain owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`.

## Blockers

- None.

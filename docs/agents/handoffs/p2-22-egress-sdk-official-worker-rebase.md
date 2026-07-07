# Handoff

Task: `docs/tasks/p2/22-egress-sdk-official-worker-rebase.md`

## Changed

- `sdk/egress/runtime.go`: moved worker session runtime into the public SDK: registration request/reply, heartbeat
  request/reply, bounded registration retry, heartbeat-NACK re-registration, ready state, final draining heartbeat, and
  a temporary assignment-loop factory seam.
- `cmd/egress/main.go`: builds SDK identity/capability values and calls `sdk/egress.Run`; the official HTTP executor
  stays in `internal/egress`.
- `internal/egress/registration.go`: reduced registration/session exports to SDK aliases and compatibility wrappers;
  kept the existing assignment loop and test frame helper for follow-on tasks.
- `sdk/egress/runtime_test.go`, `cmd/egress/main_test.go`: added SDK session runtime coverage and command wiring proof.
- `docs/tasks/p2.md`, `docs/tasks/p2/22-*`, `docs/tasks/p2/24-*`, `docs/tasks/p2/26-*`, `docs/tasks/p2/27-*`,
  `docs/tasks/p2/28-*`: recorded the user-approved split of the original oversized task 22.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `cmd/egress/main.go` constructs the worker session through `sdk/egress.Run`, not `internal/egress.Run`. | VERIFIED | `cmd/egress/main.go:265`, `cmd/egress/main.go:273` | `go test ./cmd/egress -run TestRunWorkerUsesSDKRuntime` |
| `internal/egress` no longer owns registration, heartbeat loop, bounded registration retry, or session-loss re-registration except as compatibility wrappers. | VERIFIED | `internal/egress/registration.go:30`, `internal/egress/registration.go:50`, `internal/egress/registration.go:60`, `internal/egress/registration.go:70`; SDK runtime at `sdk/egress/runtime.go:124`, `sdk/egress/runtime.go:160`, `sdk/egress/runtime.go:206` | `go test ./sdk/... ./internal/egress ./cmd/egress` |
| `sdk/egress` runtime tests cover registration retry with fresh nonces, retry cancellation, heartbeat NACK re-registration, ready state, and final draining heartbeat. | VERIFIED | `sdk/egress/runtime_test.go:72`, `sdk/egress/runtime_test.go:125`, `sdk/egress/runtime_test.go:128`, `sdk/egress/runtime_test.go:145`, `sdk/egress/runtime_test.go:215`, `sdk/egress/runtime_test.go:236` | `go test ./sdk/egress -run 'Test(RegisterWithRetry|RunReregisters)'` |
| Exact-session assignment subscription and request stream protocol remain owned by follow-on tasks 26-28, named in this task's handoff rather than left as an unowned deferral. | VERIFIED | Owners: `docs/tasks/p2/26-egress-sdk-decoded-stream-runtime-rebase.md:62`, `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md and docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`, `docs/tasks/p2/28-egress-sdk-runtime-test-migration-and-wiring-proof.md:62`; this handoff names all three. | verifier grep/file existence check |
| `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches. | VERIFIED | No matches in `sdk/egress`. | exact grep returned empty |
| Existing official executor tests still pass. | VERIFIED | Executor tests remain under `internal/egress/executor_test.go:30` and onward. | `go test ./internal/egress -run 'TestExecutor'`; `make check` |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P2 extracts NATS registration, heartbeat, assignment handling, stream framing, and error protocol into public Go Egress SDK. | partially implemented / owned | Registration and heartbeat now live in `sdk/egress/runtime.go:41`, `sdk/egress/runtime.go:87`, `sdk/egress/runtime.go:124`; assignment/stream/error protocol is owned by `docs/tasks/p2/26-egress-sdk-decoded-stream-runtime-rebase.md`, `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md and docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`, and `docs/tasks/p2/28-egress-sdk-runtime-test-migration-and-wiring-proof.md`. |
| Official Egress Worker becomes the reference implementation built on the SDK. | partially implemented / owned | `cmd/egress/main.go:265` calls `runSDKWorker`; `cmd/egress/main.go:273` calls `sdkegress.Run`; remaining runtime rebase is owned by tasks 26-28. |
| Official worker keeps outbound HTTP execution behavior. | implemented | `cmd/egress/main.go:247` still constructs `internalegress.NewExecutor`; `internal/egress/executor.go` remains the official outbound execution engine. |
| NATS registration subject is `straw.v1.control.register` and heartbeat subject is `straw.v1.control.heartbeat`. | implemented | `sdk/egress/runtime.go:60`, `sdk/egress/runtime.go:102`; subjects are SDK helpers from task 12. |
| Registration/heartbeat use protobuf `Envelope` payloads over NATS. | implemented | `sdk/egress/runtime.go:49`, `sdk/egress/runtime.go:55`, `sdk/egress/runtime.go:91`, `sdk/egress/runtime.go:97`. |
| Heartbeat NACK requires session loss and re-registration. | implemented | `sdk/egress/runtime.go:144`, `sdk/egress/runtime.go:152`, `sdk/egress/runtime.go:206`; tested by `sdk/egress/runtime_test.go:145`. |
| Assignment flow and stream sequencing/credit/error protocol. | out of scope | Owned by `docs/tasks/p2/26-egress-sdk-decoded-stream-runtime-rebase.md` and `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md and docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`. |
| Provider Adapter baseline acceptance: SDK-built worker protocol conformance and official worker on SDK E2E. | out of scope | Owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`, after tasks 26-28. |

## Verification

```sh
go test ./sdk/... ./internal/egress ./cmd/egress
grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress
make check
```

Result:

- `go test ./sdk/... ./internal/egress ./cmd/egress`: passed.
- SDK internal import grep: no matches.
- `make check`: passed (`go test ./...`, `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`).
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces).
- Live compose verification: skipped because this task only moves SDK session registration/heartbeat wiring; task 24
  owns live compose verification after the remaining runtime slices land.

## Reviewer Start Points

- `sdk/egress/runtime.go`
- `cmd/egress/main.go`
- `internal/egress/registration.go`
- `docs/tasks/p2/26-egress-sdk-decoded-stream-runtime-rebase.md`
- `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md and docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`
- `docs/tasks/p2/28-egress-sdk-runtime-test-migration-and-wiring-proof.md`

## Remaining Work

- `docs/tasks/p2/26-egress-sdk-decoded-stream-runtime-rebase.md`: move decoded assignment and stream runtime.
- `docs/tasks/p2/27-egress-sdk-raw-tunnel-runtime-rebase.md`: move raw tunnel runtime hooks; `docs/tasks/p2/31-egress-sdk-bodyref-runtime-rebase.md`: move BodyRef runtime hooks.
- `docs/tasks/p2/28-egress-sdk-runtime-test-migration-and-wiring-proof.md`: finish runtime test migration and wiring proof cleanup.
- `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`: SDK-only conformance and live compose verification.

## Blockers

- None.

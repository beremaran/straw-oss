# Handoff

Task: `docs/tasks/p2/12-egress-sdk.md`

## Changed

- Added `sdk/egress` as the public Egress worker protocol foundation: executor seam, identity/capability structs,
  registration signing, heartbeat construction, assignment admission, NATS subject helpers, and protobuf envelope
  marshal/unmarshal helpers.
- Added focused SDK tests for registration signing, heartbeat construction, assignment admission, safe subject helpers,
  and envelope round trips.
- Left `internal/egress` unchanged so the official worker behavior stays untouched for this foundation slice.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `sdk/egress` exists and exposes `Executor`, identity/capability types, registration/heartbeat helpers, assignment admission, and subject/envelope helpers sufficient for a custom worker runtime foundation. | VERIFIED | `sdk/egress/types.go:37`, `sdk/egress/types.go:42`, `sdk/egress/types.go:50`, `sdk/egress/types.go:84`, `sdk/egress/types.go:121`, `sdk/egress/types.go:157`, `sdk/egress/types.go:179`, `sdk/egress/types.go:298` | `go test ./sdk/... ./internal/egress` |
| `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches. | VERIFIED | `sdk/egress/types.go:13` imports protobuf and public proto only. | `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` exited 1 with no output. |
| SDK tests prove registration signing, heartbeat construction, assignment admission, and subject/envelope round trips. | VERIFIED | `sdk/egress/types_test.go:19`, `sdk/egress/types_test.go:61`, `sdk/egress/types_test.go:76`, `sdk/egress/types_test.go:94`, `sdk/egress/types_test.go:124` | `go test ./sdk/... ./internal/egress` |
| Existing `internal/egress` tests still pass, proving no official worker behavior changed in this foundation slice. | VERIFIED | `internal/egress` was not changed. | `go test ./sdk/... ./internal/egress`; `make check` |
| Tasks 22, 23, and 24 own the remaining Egress SDK extraction work explicitly. | VERIFIED | `docs/tasks/p2/12-egress-sdk.md:36`, `docs/tasks/p2/12-egress-sdk.md:37`, `docs/tasks/p2/12-egress-sdk.md:38`; `docs/tasks/p2.md:51` | Independent verifier checked task and board ownership. |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P2 custom Egress implementations use a public Egress SDK with protocol machinery and pluggable execution behavior. | implemented | `sdk/egress/types.go:37`, `sdk/egress/types.go:84`, `sdk/egress/types.go:121`, `sdk/egress/types.go:157`, `sdk/egress/types.go:179`, `sdk/egress/types.go:298` |
| Custom implementations are operator-configured only; no marketplace/provider billing behavior. | already existed | No marketplace/provider/billing code added; task 13 remains blocked until task 24. |
| NATS transport uses protobuf `Envelope`, not JSON. | implemented | `sdk/egress/types.go:298`, `sdk/egress/types.go:312` |
| Envelope helper preserves request/tenant/trace/deadline/protocol/attempt/payload fields. | implemented | `sdk/egress/types_test.go:124` |
| Registration subject is `straw.v1.control.register`. | implemented | `sdk/egress/types.go:179` |
| Heartbeat subject is `straw.v1.control.heartbeat`. | implemented | `sdk/egress/types.go:182` |
| Log telemetry subject is `straw.v1.control.logs`. | implemented | `sdk/egress/types.go:185` |
| Exact assignment subject is `straw.v1.executor.<worker_id>.<session_id>.assign`. | implemented | `sdk/egress/types.go:218`, `sdk/egress/types_test.go:94` |
| Stream subjects are `straw.v1.req.<request_id>.<worker_id>.<session_id>.c2e/e2c`. | implemented | `sdk/egress/types.go:233`, `sdk/egress/types_test.go:94` |
| Dot-free safe tokens are required for `request_id`, `worker_id`, and `session_id`. | implemented | `sdk/egress/types.go:201`, `sdk/egress/types_test.go:94` |
| Worker inbox prefix is `_INBOX.wrk.<worker_id>`. | implemented | `sdk/egress/types.go:191`, `sdk/egress/types_test.go:94` |
| RegisterRequest helper carries worker identity, credential, executor type, protocol version, nonce, issued-at, signature, pool refs, tags, countries, regions, IP types, ingress modes, max concurrency, software version, and initial draining. | implemented | `sdk/egress/types.go:84`, `sdk/egress/types_test.go:19` |
| Heartbeat helper carries worker ID, session ID, health, active requests, max concurrency, available capacity, draining, and worker timestamp. | implemented | `sdk/egress/types.go:121`, `sdk/egress/types_test.go:61` |
| Assignment admission rejects invalid requests, draining, unsupported mode, and exhausted capacity; otherwise accepts. | implemented | `sdk/egress/types.go:157`, `sdk/egress/types_test.go:76` |
| Follow-on official worker rebase. | out of scope | Owned by `docs/tasks/p2/22-egress-sdk-official-worker-rebase.md`. |
| Provider Adapter enum/doc cleanup. | out of scope | Owned by `docs/tasks/p2/23-executor-delegated-resolution-enum-rename.md`. |
| SDK conformance and live compose verification. | out of scope | Owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`. |
| Example custom Egress implementation. | out of scope | Owned by `docs/tasks/p2/13-example-custom-egress.md` after task 24. |

## Verification

```sh
go test ./sdk/... ./internal/egress
grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress
make check
```

Result:

- Focused tests: passed.
- Internal-import grep: no matches.
- `make check`: passed.
- Postgres-backed tests: not exercised (diff does not touch Postgres surfaces or migrations).
- Live compose verification: skipped because this task adds SDK protocol helpers only and does not touch the runtime
  request path; live SDK conformance is owned by `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`.

## Reviewer Start Points

- `sdk/egress/types.go`
- `sdk/egress/types_test.go`

## Remaining Work

- None for this task. Official worker rebase, enum/doc cleanup, SDK conformance/live verification, and the example
  custom implementation are owned by tasks 22, 23, 24, and 13 as listed above.

## Blockers

- None.

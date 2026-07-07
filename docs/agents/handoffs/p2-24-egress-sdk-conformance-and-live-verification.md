# Handoff

Task: `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`

## Changed

- `sdk/egress/conformance_test.go` (new): SDK-only conformance test. Plays the Control role by hand (subscribe/reply
  registration and heartbeat, exact-session assign, c2e/e2c stream subjects per the subscription-ordering rule in
  `docs/planning/12-nats-protocol.md`) against the public `sdk/egress` API — `Register`, `Heartbeat`,
  `NewWorker`/`Worker.Serve` — with a stub static-response `Executor`. Proves registration, heartbeat, assignment
  receipt, response streaming, and executor error mapping over a real `*nats.Conn`.
- `sdk/egress/conformance_wire_test.go` (new): a package-local, stdlib-only Core NATS wire-protocol broker used only
  by the conformance test. `sdk/egress` cannot import `internal/testutil.FakeNATSServer` (that would violate the
  task's no-`internal/*`-imports acceptance criterion), so this is a trimmed, renamed port of the same technique:
  enough of the NATS text protocol (`SUB`/`UNSUB`/`PUB`/`MSG`, with `*`/`>` wildcard matching for nats.go's mux
  inbox) for the real `nats.go` client to run request/reply and pub/sub, including genuine `nats.Msg.Respond` —
  required because `Worker.handleAssign` calls `msg.Respond` directly, which only works against a real connection.
- `docs/tasks/p2.md`, `docs/tasks/p2/24-egress-sdk-conformance-and-live-verification.md`: marked task 24 done after
  independent verification.

No changes to `sdk/egress` production code were needed — the conformance test found no API gaps.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12, two passes — the second after the
compose stack was brought back up so the verifier could check the live claim directly), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test / command |
|-----------|---------|----------------------------|--------------|
| The SDK-only conformance test proves registration, heartbeat, assignment receipt, response streaming, and executor error mapping with no `internal/*` imports. | VERIFIED | `sdk/egress/conformance_test.go:55-135` exercises `Register` (registration), `Heartbeat`, `NewWorker`/`Worker.Serve` (assignment receipt + response streaming), and a second assignment with `exec.errorMode = true` (executor error mapping), all against a real `*nats.Conn`. | `go test ./sdk/egress/... -run TestSDKConformance -v` → PASS |
| `grep -R "\"github.com/beremaran/straw/v2/internal/" sdk/egress` returns no matches, including conformance tests. | VERIFIED | `sdk/egress/conformance_test.go`, `sdk/egress/conformance_wire_test.go` import only stdlib, `nats.go`, and `strawpb`. | `grep -R "\"github.com/beremaran/straw/v2/internal/" -n sdk/egress` → no matches |
| A live request through the compose stack succeeds against the rebased `cmd/egress` binary. | VERIFIED | `cmd/egress/main.go:274` wires `sdkegress.Run(...)` as the runtime entry point. | See Verification below; verifier independently drove the same live request and inspected `docker compose logs egress` and `cmd/egress/main.go:274` itself while the stack was up. |
| Task 13 can proceed using only `sdk/egress`; any blocker discovered here is either fixed in this task or assigned to a new owning task before handoff. | VERIFIED | No API gap was found — the conformance test's stub executor completed a full assignment cycle (registration through terminal frame) using only exported `sdk/egress` identifiers. | No blocker to assign; task 13 is unblocked. |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| SDK owns NATS registration, heartbeat, assignment handling, stream framing, and error protocol; official worker is the reference implementation on the SDK (`05-component-boundaries.md`). | already existed | Built by tasks 12/22/26/27/31/28; this task adds the independent conformance proof (`sdk/egress/conformance_test.go`). |
| Registration/heartbeat envelope validation, exact-session assignment subject, c2e/e2c stream subjects, subscription-ordering rule, stream sequencing (`12-nats-protocol.md`). | already existed | Exercised end-to-end by the conformance test; the control-simulator side in `conformance_test.go:163-207` deliberately follows the documented subscribe-before-assign ordering. |
| Egress SDK test rows before feature ships (`30-testing-matrix.md`). | implemented | `sdk/egress/conformance_test.go` is the SDK-level conformance row; per-behavior protocol rows (sequence gaps, credit, cancellation, tunnel) already existed in `sdk/egress/assignment_test.go`. |
| P2 Provider Adapter Baseline acceptance tests: SDK-built worker protocol conformance (registration, assignment, stream, errors); official worker on the SDK passing the existing E2E flow; constrained error facts; no marketplace/provider billing behavior (`32-open-decisions.md`). | implemented | Conformance test covers the first and third items; the live compose request (Verification below) covers the second; the fourth was never implemented and has no work to close (P2 decision explicitly rejects it). |

## Verification

```sh
go test ./sdk/egress/... -run TestSDKConformance -v -race
grep -R "\"github.com/beremaran/straw/v2/internal/" -n sdk/egress
make check
```

Result:

- Focused test: `TestSDKConformanceRegistrationHeartbeatAssignmentStreamAndExecutorError` passed (including `-race`).
- SDK internal-import grep: no matches.
- `make check`: passed (fmt-check, `go test ./...`, `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`).
- Postgres-backed tests: not exercised; diff does not touch Postgres surfaces.
- Live compose verification: performed twice, second pass independently reproduced by the verifier agent while the
  stack was up.
  ```sh
  test -f .dev/mitm-ca/ca.pem || scripts/dev-mitm-ca.sh
  STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY='sk_live_task24_local_admin_0123456789abcdef' docker compose up -d --build
  curl -fsS http://localhost:9090/readyz && echo ready
  curl -s -H "Authorization: Bearer sk_live_task24_local_admin_0123456789abcdef" -H 'Content-Type: application/json' \
    -d '{"role":"requester"}' \
    http://localhost:8080/api/v1/config/tenants/22222222-2222-4222-8222-222222222222/api-keys
  curl -s -H "Authorization: Bearer <requester-secret>" -H 'Content-Type: application/json' \
    -d '{"method":"GET","url":"https://example.com/","timeout_ms":15000}' \
    http://localhost:8080/api/v1/requests
  ```
  Result: `status:200` with the real `example.com` HTML body (`<title>Example Domain</title>`) both times.
  `docker compose logs egress` showed only clean `connected to nats` / `health listening` / `starting run loop`
  lines, no errors. The stack's Postgres volume was reset (`docker compose down -v`) before the first run because
  the pre-existing volume held a stale bootstrap admin key from an earlier task's session; the volume was left
  intact (`docker compose down`, no `-v`) after the second run.

## Reviewer Start Points

- `sdk/egress/conformance_test.go`
- `sdk/egress/conformance_wire_test.go`
- `cmd/egress/main.go:274`

## Remaining Work

- None for task 24. Task 13 (`docs/tasks/p2/13-example-custom-egress.md`) is now unblocked and can proceed using
  only `sdk/egress`.

## Blockers

- None. Work is committed (see commit for this task).

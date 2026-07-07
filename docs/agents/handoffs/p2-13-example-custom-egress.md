# Handoff

Task: `docs/tasks/p2/13-example-custom-egress.md`

## Changed

- `examples/egress-static/main.go` (new): the example worker binary. Parses flags (`-nats-servers`,
  `-worker-id`, `-credential-id`, `-max-concurrency`, `-status`, `-body`), loads or generates an ed25519
  identity key (`STRAW_EGRESS_STATIC_PRIVATE_KEY_B64`), connects to NATS with `nats.go` directly, and
  runs `sdkegress.Run` with an `AssignmentFactory` that builds a `sdkegress.Worker` bound to the static
  executor. Uses only the standard library, `nats.go`, and `sdk/egress` — no `internal/*` import.
- `examples/egress-static/executor.go` (new): `staticExecutor`, a `sdkegress.Executor` that answers every
  decoded-HTTP assignment with the same fixed status/body — no hostname resolution, no upstream dial.
- `examples/egress-static/main_test.go` (new): integration test. Uses `internal/testutil.FakeNATSServer`
  (test-only; not part of the built binary) to play the Control role by hand — subscribe/reply
  registration and heartbeat, dispatch one `AssignRequest`/`RequestStart` over the real NATS wire protocol
  via a real `*nats.Conn` — and asserts the static `ResponseStart`/`Data`/`End` frames reach the Control
  side.
- `examples/egress-static/README.md` (new): run instructions (flags, the `public_key_b64` a real deployment
  must register against a Control credential, the `STRAW_EGRESS_STATIC_PRIVATE_KEY_B64` persistent-key
  convention) and the operator-obligations section (equivalent destination-policy enforcement for any
  implementation that starts reaching real destinations; constrained, public-safe execution facts),
  each citing the required planning docs.
- `docs/tasks/p2.md`, `docs/tasks/p2/13-example-custom-egress.md`: marked task 13 done after independent
  verification.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report (straw-task-runner workflow step 12), not from the implementer's
self-assessment.

| Criterion | Verdict | Implementation (file:line) | Proving test / command |
|-----------|---------|----------------------------|--------------|
| `examples/egress-static` builds and imports no `internal/*` packages (proven by `go list -deps` or grep). | VERIFIED | `examples/egress-static/main.go`, `executor.go` import only stdlib, `nats.go`, and `sdk/egress`. | `go build ./examples/...` → OK; `go list -deps ./examples/egress-static/ \| grep "beremaran/straw/v2/internal"` → no matches; `grep -rn "internal/" examples/egress-static/main.go examples/egress-static/executor.go` → no matches. |
| The integration test proves a request dispatched to the example executor returns the static page through the normal stream protocol. | VERIFIED | `examples/egress-static/main_test.go:29-103` drives a real NATS wire round-trip (registration, `Worker.Serve`, `AssignRequest`/`RequestStart`, `ResponseStart`→`Data`→`End`). | `go test ./examples/... -v -race` → `TestStaticExecutorServesOneAssignment` PASS. |
| The README names both operator obligations with planning-doc citations. | VERIFIED | `examples/egress-static/README.md` "Operator obligations" section cites `docs/planning/16-egress-execution.md` and `docs/planning/27-security-controls.md` for destination-policy enforcement, and `docs/planning/27-security-controls.md` for constrained public-safe execution facts; `docs/planning/05-component-boundaries.md` framing throughout. | Manual read; verifier confirmed citations present at README.md:49-69. |
| No vendor/provider names appear anywhere in the example. | VERIFIED | No named-provider strings anywhere in `examples/egress-static/`. | `grep -riE "bright ?data\|scrape\.do\|oxylabs\|smartproxy\|luminati\|zyte\|proxycrawl" examples/egress-static/` → no matches. |

## Planning-Doc Coverage

Every in-phase field/endpoint/behavior from the task's cited planning-doc sections, accounted for:

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Egress SDK and Custom Egress Implementations: operator may build a custom Egress node on `sdk/egress` supplying its own execution behavior; no named-provider integration (`05-component-boundaries.md`). | implemented | `examples/egress-static/main.go`, `executor.go` — a static-response executor built purely on `sdk/egress`, no vendor names. |
| Custom implementations are operator-configured only and assume equivalent destination-policy enforcement and constrained public-safe execution facts (`05-component-boundaries.md`). | implemented (documented) | `examples/egress-static/README.md` "Operator obligations" section. This example itself never resolves/dials so it carries no live destination-policy obligation today (`staticExecutor.Execute` never calls out); README states explicitly when that obligation begins to apply. |
| Executor-delegated mode: a custom Egress implementation that delegates upstream execution must enforce equivalent destination policy and report constrained facts (`16-egress-execution.md`). | implemented (documented) | Same README section, citing this doc directly. |
| Executor-delegated resolution (`DESTINATION_RESOLUTION_EXECUTOR_DELEGATED`): implementation resolves destinations internally, must enforce equivalent policy, reports constrained facts (`27-security-controls.md`). | implemented (documented) | Same README section, citing this doc directly. |

## Verification

```sh
go build ./examples/...
go list -deps ./examples/egress-static/ | grep "beremaran/straw/v2/internal"   # no matches
grep -rn "internal/" examples/egress-static/main.go examples/egress-static/executor.go   # no matches
go test ./examples/... -v -race
golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 ./examples/...
gofmt -l examples/egress-static/
make check
```

Result:

- Focused test: `TestStaticExecutorServesOneAssignment` passed (including `-race`).
- No-`internal/*` grep and `go list -deps`: no matches.
- `golangci-lint` on `./examples/...`: 0 issues.
- `gofmt -l`: no output (clean).
- `make check`: passed (fmt-check, `go test ./...`, `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`).
- Postgres-backed tests: not exercised; diff does not touch Postgres surfaces.
- Live compose verification: skipped (task marks this step optional; no compose stack was brought up this
  session). The task's own acceptance criteria do not require it — only the SDK-only integration test and
  the no-`internal/*` build proof are required. Recorded as an unchecked, explicitly-optional step in the
  task file's Steps list, not a silent gap.

## Reviewer Start Points

- `examples/egress-static/main.go`
- `examples/egress-static/executor.go`
- `examples/egress-static/main_test.go`
- `examples/egress-static/README.md`

## Remaining Work

- None. Nothing in this task is faked, stubbed, or deferred: the executor genuinely never dials upstream
  (so it has no destination-policy code to fake), and the README documents the obligation that begins to
  apply the moment an implementer extends it to forward anywhere real.
- No SDK friction was found: `Identity`, `Capabilities`, `Register`, `Run`, `AssignmentFactory`,
  `NewWorker`/`WorkerOptions`, and `Executor` covered everything this example needed with no
  `internal/*` reach-around.

## Blockers

- None. Work is ready to commit.

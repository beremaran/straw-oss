# Handoff

Task: `docs/tasks/p1/17-worker-loss-and-nats-outage-hardening.md`

## Changed

- `internal/control/dispatcher.go`: added pre-`RequestStart` fallback to an alternate eligible worker for assignment timeout/reject outcomes, and explicit stream-loss synthesis for closed worker streams and disconnected Control NATS.
- `internal/control/tunnel_dispatcher.go`: added the same stream-loss synthesis for CONNECT tunnel streams.
- `internal/control/*_test.go`: added focused worker-loss, NATS-outage, fallback-boundary, cooldown, and NATS error metric tests.
- `docs/tasks/p1.md` and `docs/tasks/p1/17-worker-loss-and-nats-outage-hardening.md`: marked task 17 complete after verification.

Pre-connect fallback decision: do not add `OutboundStartFrame`-based fallback after `RequestStart` in this task. Runtime fallback is only before `RequestStart`; after `RequestStart`, worker/transport loss synthesizes a terminal outcome. This keeps the P1 hardening conservative and avoids adding replay machinery or persistent queues.

## Acceptance Criteria Verdicts

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| Worker-loss and NATS-outage behavior matches Section 29 under test | VERIFIED | `internal/control/dispatcher.go:416`, `internal/control/dispatcher.go:755`, `internal/control/dispatcher.go:802`, `internal/control/tunnel_dispatcher.go:217` | `TestDispatcherWorkerLossBeforeRequestStartFallsBackToAlternateWorker`, `TestDispatcherWorkerLossAfterRequestStartSynthesizesWorkerDisconnected`, `TestDispatcherWorkerLossAfterPartialResponseSynthesizesWorkerDisconnected`, `TestTunnelWorkerLossSynthesizesWorkerDisconnected` |
| Replay/fallback boundaries are explicit and tested | VERIFIED | `internal/control/dispatcher.go:416`, `internal/control/dispatcher.go:1223` | `TestAssignmentOutboundStartDoesNotRelaxFallbackBoundary`, `TestDispatcherWorkerLossBeforeRequestStartFallsBackToAlternateWorker` |
| No persistent queue behavior is added | VERIFIED | `internal/control/dispatcher.go:416`, `internal/control/dispatcher.go:1033` | `make check`; changed files are limited to Control dispatcher/tests and task docs |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Section 29: NATS unavailable makes new request dispatch fail `transport_unavailable` | implemented | `internal/control/dispatcher.go:486`, `internal/control/dispatcher_test.go:766`, `internal/control/dispatcher_test.go:814` |
| Section 29/09: in-flight streams fail by timeout/loss semantics | implemented | `internal/control/dispatcher.go:755`, `internal/control/dispatcher.go:802`, `internal/control/tunnel_dispatcher.go:217`, `internal/control/tunnel_dispatcher.go:225` |
| Section 09: fallback allowed before `RequestStart` | implemented | `internal/control/dispatcher.go:416`, `internal/control/dispatcher_test.go:838` |
| Section 09: after `RequestStart`, no pre-connect fallback via `OutboundStartFrame` in this slice | implemented | `internal/control/lifecycle_test.go:90`, decision documented above |
| Section 09: terminal outcomes close request stream and ignore late frames | already existed | `internal/control/dispatcher.go:927`, `internal/natsx/stream.go:101` |
| Section 12: Core NATS only, no durable queue/redelivery | already existed | No JetStream/persistent queue changes; `make check` passed |
| Section 11: repeated failures contribute to cooldown | implemented | `internal/control/dispatcher.go:986`, `internal/control/dispatcher_test.go:897`, `internal/control/tunnel_dispatcher_test.go:153` |

## Verification

```sh
go test ./internal/control -run 'TestDispatcher(WorkerLossBeforeRequestStart|AssignmentNATSOutage|WorkerLossAfter|InFlightNATSDisconnect|NATSUnavailable|AssignmentTimeout|StreamProtocolError)|TestTunnel(WorkerLoss|NATSDisconnect|IdleTimeout)|TestAssignment'
make check
```

Result:

- Postgres-backed tests: not exercised; diff does not touch Postgres surfaces or migrations.
- Live compose verification: skipped because another project already bound host port `6379` (`financetracker-redis-1`), blocking Straw Redis startup. The partial Straw compose stack was brought down with `docker compose down`.

## Reviewer Start Points

- `internal/control/dispatcher.go`
- `internal/control/dispatcher_test.go`
- `internal/control/tunnel_dispatcher.go`
- `internal/control/tunnel_dispatcher_test.go`
- `internal/control/lifecycle_test.go`

## Remaining Work

- None.

## Blockers

- None.

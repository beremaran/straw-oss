# Handoff

Task: `docs/tasks/p1/05-raw-connect-tunnel.md`

## Changed

- Added static `control.server.connect_enabled` / `connect_port` config, defaulting enabled CONNECT ingress to port 8082.
- Added a raw CONNECT listener in `cmd/control/main.go` and a CONNECT-only handler that authenticates `Proxy-Authorization`, normalizes `host:port`, hijacks the connection, and dispatches `IngressTypeConnect`.
- Added Control tunnel dispatch over existing `StreamFrame` `DataFrame`/`CreditFrame`/`CancelFrame` semantics.
- Added official worker raw tunnel execution: raw tunnel assignment support, destination-policy validated TCP dial, bidirectional byte streaming, upload/download credit grants, cancellation, and idle handling.
- Added focused tests for CONNECT config/listener gating, target normalization, auth/method rejection, raw tunnel assignment, destination-policy dial/deny, upload credit blocking, idle cancellation, and client disconnect cancellation.
- Updated the stale task-22 handoff IDNA note to point at `docs/tasks/p1/21-idna-hostname-support.md`.

## Acceptance Criteria Verdicts

Filled from the independent verifier's report.

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| CONNECT is accepted only on the P1 CONNECT ingress. | VERIFIED | `internal/control/connect_handler.go:35`, `internal/control/request.go:188`, `internal/control/proxy_handler.go:129`, `cmd/control/main.go:242`, `cmd/control/main.go:721` | `TestConnectHandlerRejectsNonConnectBeforeDispatch`, `TestHandlerCONNECTRejected`, `TestBuildConnectHandlerOnlyWhenEnabled` |
| Tunnel bytes obey existing NATS credit and cancellation rules. | VERIFIED | `internal/control/tunnel_dispatcher.go:171`, `internal/control/tunnel_dispatcher.go:286`, `internal/control/tunnel_dispatcher.go:312`, `internal/control/tunnel_dispatcher.go:322`, `internal/control/tunnel_dispatcher.go:332`, `internal/egress/loop.go:316`, `internal/egress/loop.go:393` | `TestTunnelUploadGateWaitsForCreditGrant`, `TestTunnelIdleTimeoutSendsCancel`, `TestTunnelClientEOFSendsCancel`, `TestWorkerDownloadCreditGatesResponseData`, `TestEvaluateAssignmentAcceptsRawTunnelMode` |
| Future-work tunnel types are not generalized into this task. | VERIFIED | `internal/control/connect_handler.go:35`, `internal/control/dispatcher.go:994`, `internal/egress/executor.go:405` | `TestExecutorOpenTunnelUsesDestinationPolicyDial`, `TestEvaluateAssignmentAcceptsRawTunnelMode`, `make check` |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| Section 15: REST decoded transport rejects `CONNECT`. | already existed | `internal/control/request.go:188`, `internal/control/handler_test.go:88` |
| Section 15: raw CONNECT accepted only by P1 CONNECT ingress. | implemented | `internal/control/connect_handler.go:35`, `cmd/control/main.go:242` |
| Section 12: c2e carries tunnel upload `DataFrame`, cancel, and response/download credit. | implemented | `internal/control/tunnel_dispatcher.go:332`, `internal/control/tunnel_dispatcher.go:327`, `internal/control/tunnel_dispatcher.go:463` |
| Section 12: e2c carries tunnel download `DataFrame`, upload credit, terminal frames. | implemented | `internal/egress/loop.go:393`, `internal/egress/loop.go:425`, `internal/egress/loop.go:383` |
| Section 12: byte-credit flow control and 15s frame idle timeout. | implemented | `internal/control/tunnel_dispatcher.go:172`, `internal/control/tunnel_dispatcher.go:312`, `internal/egress/loop.go:402` |
| Section 27: CONNECT target host/port normalization and destination deny enforcement. | implemented | `internal/control/connect_handler.go:113`, `internal/egress/executor.go:201`, `internal/egress/executor.go:209` |
| Section 27: IDNA/punycode normalization. | out of scope | Owned by `docs/tasks/p1/21-idna-hostname-support.md` |
| Section 10: route `ingress_type=connect` and worker capability routing. | already existed | `internal/control/dispatcher.go:323`, `internal/control/routing_test.go:190` |
| Section 28: port 8082 raw CONNECT proxy. | implemented | `internal/config/config.go:78`, `cmd/control/main.go:242` |

## Verification

```sh
go test ./internal/control ./internal/egress ./cmd/control ./internal/config
make check
```

Result:

- Focused tests: pass.
- `make check`: pass (`go test ./...` and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`).
- Postgres-backed tests: not exercised; diff does not touch Postgres files or migrations.
- Live compose verification: skipped because `docker compose ps` showed no running services.

## Reviewer Start Points

- `internal/control/connect_handler.go`
- `internal/control/tunnel_dispatcher.go`
- `internal/egress/loop.go`
- `internal/egress/executor.go`
- `cmd/control/main.go`

## Remaining Work

- None.

## Blockers

- None.

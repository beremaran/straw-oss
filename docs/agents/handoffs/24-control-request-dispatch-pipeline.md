# Handoff

Task: `docs/tasks/p0/24-control-request-dispatch-pipeline.md`

## Changed

- `internal/control/dispatcher.go`: added the P0 Control request dispatcher: config snapshot capture, Redis-backed rate/quota admission, routing, destination policy resolution, exact-session NATS assignment, `RequestStart`/body frame publishing, e2c stream validation, response buffering, and canonical error mapping.
- `internal/control/handler.go`: removed the synthetic success response path and routes validated REST requests into a dispatcher.
- `cmd/control/main.go`: wires the built Control binary to the live dispatcher with NATS, `ConfigCache`, `WorkerRegistry`, Redis rate/quota admission, and Redis sticky sessions.
- `internal/control/request.go`: preserves routing hints and requested fingerprint after validation for the dispatcher.
- `internal/control/destination_policy.go`: explicit allowed CIDRs now lift matching default-deny policy flags, needed for real local loopback e2e tests while keeping default-deny behavior unless an allow rule exists.
- `internal/control/dispatcher_test.go`: added route no-match/unavailable, 429 retry-after, response-size, and live Control -> NATS -> Egress -> upstream round-trip tests.

## Verification

```sh
go test ./internal/control ./internal/egress ./cmd/...
make check
```

Result: pass. `make check` ran `go test ./...` and `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` with `0 issues`.

## Reviewer Start Points

- `internal/control/dispatcher.go`
- `internal/control/dispatcher_test.go`
- `cmd/control/main.go`

## Remaining Work

- `docs/tasks/p0/24-control-request-dispatch-pipeline.md`: implement fallback/replay attempts before the `RequestStart` boundary. Current dispatcher returns the pre-start assignment failure instead of replaying another eligible exact session.
- `docs/tasks/p0/24-control-request-dispatch-pipeline.md`: send sequenced `CancelFrame`s on client disconnect, deadline, admin cancel, shutdown, and obsolete fallback attempts.
- `docs/tasks/p0/24-control-request-dispatch-pipeline.md`: replenish e2c byte credit while buffering response frames. Current implementation validates initial download credit and response-size limit but does not publish `CreditFrame`s.
- `docs/tasks/p0/24-control-request-dispatch-pipeline.md`: add the remaining focused tests for assignment timeout, fallback before `RequestStart`, stream protocol errors, and cancellation.

## Blockers

- None.

# Handoff

Task: `docs/tasks/p0/17-worker-registration-heartbeat-over-nats.md`

## Changed

- **`internal/config/config.go`** — Added `HeartbeatIntervalMs` (default 5000ms) and `CredentialID` (required) fields to `EgressConfig` so the egress binary can be configured for NATS registration.
- **`internal/config/config_test.go`** — Updated "valid" egress config to include `credential_id`; added "missing credential id" test case.
- **`cmd/control/main.go`** — Extracted `buildWorkerRegistry()` to create `WorkerRegistry` + `WorkerCredentialStore` outside `buildControlMux`. Wired `control.SetupWorkerDiscoverySubscriptions(natsConn, workerRegistry)` into `runControl` after the mux is built and before HTTP serving starts.
- **`cmd/egress/main.go`** — Replaced the stub "connect and wait" entry point with a real run loop: generates an ed25519 keypair, builds `egress.Identity` and `egress.Capabilities` from config, and calls `egress.Run(ctx, natsConn, id, caps, heartbeatInterval)`. Handles SIGINT/SIGTERM via the existing `egress.Run` draining path.
- **`internal/control/handler_test.go`** — Extracted `"test"` string to `handlerTestMessage` constant (pre-existing goconst lint fix).
- **`internal/control/worker_nats_test.go`** — Extracted `"test"` string to `workerTestVersion` constant (pre-existing goconst lint fix).

## Verification

```sh
make check
```

Result: all tests pass, lint clean (0 issues).

## Reviewer Start Points

- `cmd/egress/main.go` — run loop wiring (identity generation, capabilities, egress.Run call)
- `cmd/control/main.go` — `buildWorkerRegistry()` extraction and `SetupWorkerDiscoverySubscriptions` call
- `internal/config/config.go` — new `EgressConfig` fields

## Remaining Work

- None.

## 2026-07-04 status reconciliation

- The implementation, wiring, tests, and this handoff were all already in place, but the task file's header still
  read `Status: not started` and its step boxes were unchecked (the prior run never flipped them; the board already
  marked #17 done). Verified against actual code — `SetupWorkerDiscoverySubscriptions` wired at
  `cmd/control/main.go:135`, `egress.Run` loop in `cmd/egress/main.go`, integration coverage in
  `internal/control/worker_nats_test.go` — then set `Status: done` and checked the completed steps. Focused tests
  and `make check` pass (0 lint issues). No code changes.

## Blockers

- None.

# Handoff

Task: `docs/tasks/p0/37-structured-json-logging.md`

## Changed

- Added `internal/logging` (new package): `logging.New(service string) *slog.Logger` builds a stdlib `log/slog`
  JSON handler writing to stdout, with a `ReplaceAttr` that renames slog's default `time` key to `timestamp` to
  match `docs/planning/23-observability.md`'s field name. `logging.NewHandler(io.Writer)` is exposed separately so
  tests can assert on emitted JSON without capturing stdout.
- `cmd/control/main.go`: `main()` now calls `slog.SetDefault(logging.New("control"))` before `run()`, so every
  subsequent `slog` call carries `service=control`. Every `log.Printf`/`log.Fatalf` call site converted to
  `slog.Info`/`slog.Warn`/`slog.Error` with structured attributes (e.g. `"error", err`, `"addr", addr`,
  `"source_env", ...`) instead of `%v`/`%s` interpolation. The final fatal path (`main()`) now does
  `slog.Error(...)` + `os.Exit(1)` instead of `log.Fatalf`.
- `cmd/egress/main.go`: same pattern with `service=egress`. The run-loop-start log now carries `worker_id` as a
  structured attribute (internal log only, per `docs/planning/23`).
- `internal/control/invalidation_redis.go`: the one `log.Printf` call (invalid invalidation payload) converted to
  `slog.Warn` with `tenant_id` and `error` attributes. This file has no logger of its own — it relies on the
  process-wide default set by `cmd/control/main.go`'s `slog.SetDefault`, which is correct since it only ever runs
  inside the control binary.
- Test: `internal/logging/logging_test.go` — `TestHandlerEmitsSingleLineJSONWithRequiredKeys` writes one log record
  through the handler into a buffer and asserts: exactly one line, valid JSON, `service`/`timestamp`/`level`/`msg`
  keys present, and that ad hoc attributes (`request_id`, `tenant_id`) round-trip.

## Contextual attributes

Field availability at the converted call sites is narrow: all of them are startup/lifecycle logging in `main()`
paths (connect, bind, bootstrap, shutdown) and one Redis-subscriber payload-parse error — none of the existing
call sites had `request_id`, `tenant_id`, or `error_code` in scope except the invalidation subscriber's
`tenant_id`. `worker_id` appears once, in egress's run-loop-start log, which is an internal-only log line (never
returned in an HTTP response). No request-path logging was added; the task's Out of Scope explicitly excludes new
per-request debug logging.

## Redaction check (docs/planning/27)

Grepped the diff for pepper/secret/credential/private-key/signed-url material: no log call in this diff logs a
secret value. The two credential-adjacent lines log only environment variable *names*
(`control.BootstrapSystemAdminEnvVar`, `control.DevWorkerIDEnvVar`), not their values. Egress's "connected to
nats" log uses `natsConn.ConnectedUrlRedacted()`, which was already redacting credentials before this change.

## Verification

```sh
go test ./cmd/... ./internal/control ./internal/logging
make check
```

Result: all pass, including `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` (0 issues) via
`make check`.

## Reviewer Start Points

- `internal/logging/logging.go`
- `internal/logging/logging_test.go`
- `cmd/control/main.go` (`main`, and every converted call site)
- `cmd/egress/main.go` (`main`, `run`, `runWorker`)
- `internal/control/invalidation_redis.go` (`applyMessage`)

## Remaining Work

- None for this task's scope. **[Update 2026-07-06: Control-local `log_events` ingestion is now implemented by
  `docs/tasks/p1/20-log-events-ingestion.md`; Egress logs over NATS remain owned by
  `docs/tasks/p1/27-egress-log-events-nats-transport.md`.]**

## Blockers

- None.

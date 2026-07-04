# 37 - Structured JSON Logging

Status: not started

## Objective

Emit structured JSON logs from both services per `docs/planning/23`: `service`, `timestamp`, `level` on every line,
plus `request_id`, `tenant_id`, and `error_code` where available; `worker_id` only in internal logs. Use the stdlib
`log/slog` JSON handler.

## Context (gap being closed)

The 2026-07-04 review follow-up found no structured logging anywhere: `log/slog` is unused and all logging goes
through plain `log.Printf` in `cmd/control/main.go`, `cmd/egress/main.go`, and
`internal/control/invalidation_redis.go`. `docs/planning/23` requires structured JSON logs from all services. The
call-site count is small, so this is a mechanical conversion plus a `service` attribute per binary. Shipping logs to
the ClickHouse `log_events` table is a separate, deferred concern owned by `docs/tasks/p1/20-log-events-ingestion.md`.

## Required Planning Docs

- `docs/planning/23-observability.md` (log field requirements)
- `docs/planning/27-security-controls.md` (log redaction: no secrets, key material, NATS credentials, or signed URLs)

## Prerequisites

- Task 01 completed (binaries exist).

## Out of Scope

- Do not build the ClickHouse `log_events` ingestion pipeline (owned by `docs/tasks/p1/20-log-events-ingestion.md`).
- Do not add new log lines beyond converting existing ones and wiring the handler; per-request debug logging is not a
  P0 requirement.
- Do not add a logging dependency; stdlib `log/slog` only.

## Expected Files

- Modify: `cmd/control/main.go` (construct a `slog` JSON logger with `service=control`; convert `log.Printf` sites).
- Modify: `cmd/egress/main.go` (same with `service=egress`).
- Modify: `internal/control/invalidation_redis.go` (log through `slog`).
- Test: a small test asserting the configured handler emits single-line JSON with the required base keys.

## Steps

- [ ] Read all required planning docs.
- [ ] Set a `slog` JSON handler as each binary's logger with a constant `service` attribute; route stdlib `log`
      output through it (`slog.SetDefault` covers `log.Printf` callers).
- [ ] Convert the existing `log.Printf`/`log.Fatal` sites to leveled `slog` calls, attaching `request_id`,
      `tenant_id`, `error_code`, or `worker_id` attributes where the calling code already has them.
- [ ] Confirm no log line can carry secret material per `docs/planning/27` (key secrets, pepper, NATS credentials,
      signed URLs).
- [ ] Run the focused tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./cmd/... ./internal/control`
- `make check`

## Acceptance Criteria

- Both binaries emit only single-line JSON log records carrying `service`, `level`, and a timestamp; contextual
  fields appear where available.
- `worker_id` appears only in internal (Control/Egress service) logs, never in client-facing responses.
- No secret values are logged; the `log_events` ClickHouse deferral names
  `docs/tasks/p1/20-log-events-ingestion.md` as owner.

## Handoff Notes

- Document the handler setup, the `service` values, and which call sites carry contextual attributes.

## Stop Conditions

- Stop before building log shipping/ingestion.
- Stop if a deferral would have no owning task file.

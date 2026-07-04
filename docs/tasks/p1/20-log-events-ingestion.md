# 20 - Log Events ClickHouse Ingestion

Status: not started

## Objective

Ship structured service logs to the ClickHouse `log_events` table defined in `docs/planning/22`, using the same
async, bounded, non-blocking write discipline as `request_events`.

## Context

This is the owning follow-up for the `log_events` deferral recorded by
`docs/tasks/p0/32-request-outcome-and-worker-audit-telemetry.md` and
`docs/tasks/p0/37-structured-json-logging.md`. P0 requires emitting structured JSON logs (`docs/planning/23`) but no
P0 planning item requires shipping them to ClickHouse; the `log_events` schema in `docs/planning/22` carries no phase
marker, so ingestion is placed here alongside the other P1 telemetry tasks (11-13). If this placement is wrong,
reconcile via `docs/planning/a-reconciliation-notes.md` before starting.

## Required Planning Docs

- `docs/planning/22-canonical-clickhouse-schema.md` (`log_events` row shape)
- `docs/planning/23-observability.md` (log fields)
- `docs/planning/27-security-controls.md` (redaction invariants)
- `docs/planning/21-state-and-storage.md` (ClickHouse role)

## Prerequisites

- P0 task 14 completed (bounded async ClickHouse writer).
- P0 task 37 completed (structured `slog` logging exists to tee from).

## Out of Scope

- Do not build log search/read APIs (P1 tasks 11-12 own telemetry reads).
- Do not capture request/response payloads.

## Expected Files

- Create: a `slog.Handler` tee that enqueues `log_events` rows onto a bounded queue feeding the existing ClickHouse
  writer.
- Modify: `cmd/control/main.go`, `cmd/egress/main.go` (install the tee when ClickHouse is configured).
- Test: handler tee test (row shape, drop-on-full, no blocking) and redaction test.

## Steps

- [ ] Read all required planning docs.
- [ ] Map `slog` records to `log_events` rows (`service`, `level`, `message`, contextual IDs, `extra`).
- [ ] Enqueue asynchronously with a bounded queue; drop oldest non-critical entries on overflow or ClickHouse outage,
      never blocking the caller.
- [ ] Verify redaction: no secret material can reach `log_events`.
- [ ] Run the focused tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/... ./cmd/...`
- `make check`

## Acceptance Criteria

- Structured logs appear as `log_events` rows with the `docs/planning/22` shape when ClickHouse is configured.
- A ClickHouse outage never blocks or fails the logging caller; overflow drops are bounded and observable.
- Redaction invariants hold for logged values.

## Handoff Notes

- Document the queue bounds, drop policy, and the fields mapped into `extra`.

## Stop Conditions

- Stop before adding read APIs or payload capture.
- Stop if a deferral would have no owning task file.

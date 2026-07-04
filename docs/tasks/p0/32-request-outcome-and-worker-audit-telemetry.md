# 32 - Request-Outcome Telemetry and Worker/Audit ClickHouse Writes

Status: done

## Objective

Record the real request outcome (status, error, timings, sizes) into ClickHouse `request_events` after dispatch, and
add the `worker_events` and `config_audit_events` write paths defined in the canonical schema.

## Context (gap being closed)

The 2026-07-04 review found `request_events` rows are enqueued once, pre-dispatch (`handler.go`), with hardcoded
outcome fields (`upstream_status = 200`, `error_code = ""`, all timings and `response_size_bytes = 0`), and are never
updated with the real result — so every persisted telemetry row is a placeholder. The `worker_events`,
`config_audit_events`, and `log_events` tables in `deploy/docker/clickhouse-schema.sql` have no writer. Task 14's
explicit criteria (redaction, sanitization, bounded queue, outage tolerance) are met and must be preserved; this task
adds outcome accuracy and the two missing write paths.

## Required Planning Docs

- `docs/planning/22-canonical-clickhouse-schema.md` (row shapes for `request_events`, `worker_events`,
  `config_audit_events`)
- `docs/planning/23-observability.md` (what outcomes must be recorded)
- `docs/planning/27-security-controls.md` (redaction invariants to preserve)
- `docs/planning/30-testing-matrix.md` (ClickHouse and Audit rows)

## Prerequisites

- Task 14 completed (async bounded writer, redaction, outage behavior).
- Task 24 completed (dispatch produces the outcome).
- Task 08 / Task 17 completed (worker registration/heartbeat events source `worker_events`).
- Task 20 completed (config writes source `config_audit_events`).

## Out of Scope

- Do not implement telemetry read APIs or dashboards (P1).
- Do not implement payload capture (P2).
- Do not build the `log_events` ingestion pipeline; the owning follow-up is
  `docs/tasks/p1/20-log-events-ingestion.md`.

## Expected Files

- Modify: `internal/control/request_metadata.go` (emit the `request_events` row with the real outcome; use a
  two-phase or finalize-on-completion approach rather than a pre-dispatch placeholder).
- Modify: `internal/control/handler.go` and/or `internal/control/dispatcher.go` (pass the dispatch outcome — status,
  error code/category, timeout type, sizes, timings — to the writer; on failure still emit with the failure fields).
- Create: worker-event and config-audit ClickHouse sinks (extend `request_metadata.go` or add
  `internal/control/telemetry_events.go`).
- Modify: `cmd/control/main.go` (wire the worker/audit sinks; the worker registry and config writer call them).
- Test: `internal/control/request_metadata_test.go` and new worker/audit-event tests.

## Steps

- [x] Read all required planning docs.
- [x] Emit the `request_events` row after dispatch with the real `upstream_status`/`client_status`,
      `error_code`/`error_category`/`timeout_type`, `request_size_bytes`/`response_size_bytes`, and
      `routing_ms`/`assignment_ms`/`egress_ms`/`total_ms`; on a dispatch failure, emit the row with the canonical
      failure fields instead of a synthetic success.
- [x] Preserve the existing sanitization (drop query/userinfo/fragment) and header redaction; keep the write async,
      bounded, and non-blocking on outage.
- [x] Add a `worker_events` write path fed by registration/heartbeat/disable/drain transitions.
- [x] Add a `config_audit_events` write path mirroring `config_audit_source` writes (already redacted upstream).
      Partial: covers every `recordAudit`/`AuditStore.Record` call site (tenant, API key, worker credential,
      routing/deny/injection/pool config, worker admin, request cancel) with tenant/actor/resource/action, but
      does not thread `field_path`/`old_value_json`/`new_value_json`/`config_version` from the separate
      `insertConfigAudit`/`writeTenantConfig` Postgres path — see the task 32 handoff's Remaining Work.
- [x] Document the `log_events` deferral, naming `docs/tasks/p1/20-log-events-ingestion.md` as the owning follow-up.
- [x] Add tests for: a completed request producing a `request_events` row with real status/timings/sizes; a failed
      request producing a row with the canonical `error_code`/category; `worker_events` and `config_audit_events`
      rows written; transport unaffected by a sink outage (fake sink); redaction still holds.
- [x] Run the focused tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- A completed request produces a `request_events` row reflecting the actual upstream status, timings, and byte sizes;
  a failed request records the canonical error code/category rather than a synthetic 200.
- `worker_events` and `config_audit_events` receive rows on the corresponding transitions/writes.
- ClickHouse write failure never fails request transport; sanitization and redaction are preserved.

## Handoff Notes

- Document the outcome-capture point in the pipeline and the fields written per table.
- List fields intentionally omitted and the `log_events` deferral's owning task
  (`docs/tasks/p1/20-log-events-ingestion.md`).

## Stop Conditions

- Stop before adding telemetry read APIs or payload capture.
- Stop if recording an outcome field would require exposing a redacted/secret value.
- Stop if a deferral would have no owning task file.

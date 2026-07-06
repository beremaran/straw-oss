# 50 - Config Audit Event `field_path` Population

Status: done

## Objective

Config audit events written to Postgres `config_audit_source` and mirrored to ClickHouse `config_audit_events`
carry a meaningful `field_path` (or a deliberate, documented sentinel) rather than an always-empty string, so the
`field_path` column and the `/changes` audit history satisfy the `docs/planning/26` "Config Audit Change" shape.
After this task, an auditor can tell from an audit row which field(s) changed, not only the whole-object before/after
JSON.

## Context (gap being closed)

The 2026-07-07 live compose verification (`docs/tasks/p0/46-live-compose-verification.md`, surface 44) confirmed
that audit rows now carry `config_version`, `old_value_json`, and `new_value_json` (task 44) with secrets redacted —
but `field_path` is **empty on every row**, for both `create` and `update` actions.

Current-code evidence:

- `internal/control/audit.go:127-159` — `recordAudit` accepts a `fieldPath string` parameter and threads it into
  `AuditRecord.FieldPath`, but every call site passes `""`:
  `grep -n 'recordAudit(' internal/control/*.go` shows ~15 calls in `admin_handlers.go` and
  `config_admin_handlers.go`, all with an empty `fieldPath` argument (e.g. `admin_handlers.go:220`, `:341`, `:415`).
- `docs/planning/26-config-management-api-surface.md:404` — the canonical "Config Audit Change" object carries
  `"field_path": "match_conditions.target_host"` alongside per-field `old_value_json`/`new_value_json`, i.e. the
  spec models a field-scoped change, not only a whole-object diff.
- `docs/planning/22-canonical-clickhouse-schema.md:90` — `config_audit_events.field_path String` is a P0 column.

Task 44 (P0) closed the `config_version`/`old_value_json`/`new_value_json` half of the 2026-07-05 audit gap using a
whole-object diff and left `field_path` empty; that remnant of the original gap has had no owning task until now.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` — the "Config Audit Change" structure (lines ~392-411) and the
  P0 `/changes` endpoint row (line ~74).
- `docs/planning/22-canonical-clickhouse-schema.md` — `config_audit_events` schema (lines ~77-102), the `field_path`
  column.

## Prerequisites

- Task 44 done (audit enrichment: config_version + old/new value JSON + redaction + double-write prevention). Done.

## Out of Scope

- Do not change secret redaction, the `SkipPostgres` double-write prevention, or the `config_version`/old/new value
  wiring — those are built (task 44) and verified live.
- Do not build the P1 rollback flow (`docs/planning/26` "P1 Config Resource Schemas"); this task is audit-read
  fidelity only.
- Do not remove the whole-object `old_value_json`/`new_value_json`; `field_path` complements them, it does not
  replace them.

## Expected Files

- Modify: `internal/control/admin_handlers.go` and `internal/control/config_admin_handlers.go` — pass a meaningful
  `fieldPath` at each `recordAudit` call site (e.g. the changed field for single-field mutations, or a documented
  whole-resource sentinel like `"*"` when the mutation is a full-object upsert).
- Modify (if needed): `internal/control/audit.go` — helper to derive/validate `fieldPath`, keeping call sites tidy.
- Test: `internal/control/audit_test.go` (and/or the handler tests) asserting non-empty `field_path` on a
  representative update, and the documented sentinel on a whole-object upsert.

## Steps

- [x] Read the required planning doc sections listed above.
- [x] Decide the `field_path` convention: per-field path when a mutation targets one field, and a documented
      sentinel (e.g. `"*"`) for whole-object upserts. Record the convention in the handoff.
- [x] Update every `recordAudit` call site in `admin_handlers.go` and `config_admin_handlers.go` to pass a
      non-empty `fieldPath` per the convention.
- [x] Add tests asserting `field_path` is non-empty (and correct) for an update and for a whole-object upsert, in
      both the `InMemoryAuditStore` and the ClickHouse-mirror record path (`AuditRecord.FieldPath`).
- [x] Run focused tests (`go test ./internal/control`), then `make check`.
- [x] If the compose stack is available, re-run the surface-44 live check: perform a config update and confirm the
      ClickHouse `config_audit_events` row now carries a non-empty `field_path`.
- [x] Update `docs/tasks/p0/46-live-compose-verification.md`'s surface-44 note to record that `field_path` is now
      populated, naming this task.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control`
- `make check`

## Acceptance Criteria

- `grep -n 'recordAudit(' internal/control/*.go` shows no call passing an empty-string `fieldPath` (every call
  passes a per-field path or the documented sentinel).
- A config update produces an audit row with a non-empty `field_path`, proven by a unit test that fails against the
  current all-empty behavior.
- The chosen convention (per-field path vs sentinel for whole-object) is documented in the handoff and applied
  consistently across all config_type write paths.
- `config_audit_events.field_path` is non-empty for a live update if compose was used; otherwise the handoff records
  live-pending with this task named as owner.

## Handoff Notes

- Record the `field_path` convention and why (per-field vs whole-object sentinel), and whether the planning-doc
  per-field model is fully met or approximated.
- Record whether the live surface-44 re-check was run and its result.

## Stop Conditions

- Stop if honoring the per-field `docs/planning/26` model would require restructuring the whole-object diff task 44
  built (that is a larger change — ask before expanding scope).
- Stop if a deferral would have no owning task file.

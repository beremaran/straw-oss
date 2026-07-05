# 44 - Config Audit Event Enrichment

Status: done

## Objective

Populate `config_version`, `field_path`, `old_value_json`, and `new_value_json` on `config_audit_events` rows, with
secret redaction, so the ClickHouse audit trail matches the canonical schema instead of shipping those columns empty.

## Context (gap being closed)

The 2026-07-05 P0 verification audit confirmed `internal/control/audit.go` (`auditStoreWithEvents.Record`) enqueues
`ConfigAuditEvent` rows with only tenant/actor/resource/action populated — the struct and the ClickHouse schema
(`deploy/docker/clickhouse-schema.sql`, `docs/planning/22-canonical-clickhouse-schema.md`) already carry
`field_path`/`old_value_json`/`new_value_json`, but the upstream `AuditRecord` never supplies them. Task 32's handoff
flagged this with "no existing task file owns this specific enrichment." This task is that owner. It also unblocks
P1 rollback (`docs/tasks/p1/07-config-rollback-api.md`), which restores values from audit source records.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (Config Audit Event schema, lines ~395-411; secret redaction)
- `docs/planning/22-canonical-clickhouse-schema.md` (`config_audit_events` table and the secret-redaction rule)

## Prerequisites

- Task 32 completed (writer + wrapper wired).

## Out of Scope

- Do not build the P1 rollback API; only produce the records it will need.
- Do not add read/query APIs for the enriched fields (P1 telemetry read APIs own that).

## Expected Files

- Modify: `internal/control/audit.go` (`AuditRecord` carries the new fields; wrapper passes them through).
- Modify: the config write handlers (`config_admin_handlers.go`, `admin_handlers.go`) to supply per-write
  `config_version` and old/new values (field-level `field_path` where a single field changed; whole-resource JSON with
  empty `field_path` is an acceptable P0 floor if recorded consistently — state the choice in the handoff).
- Add: a shared redaction helper that classifies secret fields (API key material, worker credential secrets) and
  replaces their values before serialization, per `docs/planning/22`.
- Test: a config write produces an event row with correct version and old/new JSON; a secret-bearing write produces a
  redacted row (assert the secret value never appears in the serialized event).

## Steps

- [x] Read all required planning docs.
- [x] Extend `AuditRecord` and the `auditStoreWithEvents` wrapper.
- [x] Thread `config_version` and old/new values through every config write path that records an audit entry
      (routing, deny, injection, fingerprint, pools, tenants, API keys, worker credentials).
- [x] Implement and apply secret redaction before serialization.
- [x] Add the tests listed in Expected Files.
- [x] Run focused tests, then `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control`
- `make check`

## Acceptance Criteria

- Every config write's `config_audit_events` row carries the post-write `config_version` and non-empty
  `old_value_json`/`new_value_json` (old empty only for create; new empty only for delete).
- Secret fields never appear unredacted in any event row (proven by test).
- No config write path is left emitting the bare four-field row (grep `Enqueue(ConfigAuditEvent` and account for
  every caller).

## Handoff Notes

- State the field-level vs whole-resource JSON decision and list the redacted field classes.

## Stop Conditions

- Stop if redaction classification for a field is genuinely ambiguous in the planning docs; ask instead of guessing.
- Stop if a deferral would have no owning task file.

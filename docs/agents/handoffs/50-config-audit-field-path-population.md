# Handoff

Task: `docs/tasks/p0/50-config-audit-field-path-population.md`

## Summary

Config audit writes now always carry a non-empty `field_path`. Call sites pass the whole-resource sentinel `*`; when
both old and new JSON values are present, `recordAudit` refines the sentinel into a stable, comma-separated list of
changed field paths before writing to the audit store and ClickHouse mirror.

## Changed

- `internal/control/audit.go` — added `auditFieldPathAll = "*"`, `deriveFieldPath`, and the tiny JSON-object diff.
  Non-diffable changes (create/delete/no-op/non-object) keep `*`; update diffs become sorted paths like
  `match_conditions.target_host,priority`.
- `internal/control/admin_handlers.go`, `internal/control/config_admin_handlers.go`,
  `internal/control/request_admin_handlers.go`, `internal/control/worker_handlers.go` — every `recordAudit` caller now
  passes `auditFieldPathAll`, not `""`.
- `internal/control/audit_test.go` — added coverage for derived update paths and sentinel fallback through both the
  in-memory audit store and ClickHouse mirror event capture.
- `docs/tasks/p0/46-live-compose-verification.md` and
  `docs/agents/handoffs/44-config-audit-event-enrichment.md` — updated the stale surface-44 gap notes.

## Acceptance Criteria Verdicts

From the independent verifier (fresh agent, task file + diff only):

| Criterion | Verdict | Implementation (file:line) | Proving test |
|-----------|---------|----------------------------|--------------|
| `grep -n 'recordAudit(' internal/control/*.go` shows no call passing an empty-string `fieldPath`; every call passes per-field path or sentinel. | VERIFIED | `internal/control/audit.go:21`; `internal/control/admin_handlers.go:220`; `internal/control/config_admin_handlers.go:291`; `internal/control/request_admin_handlers.go:46`; `internal/control/worker_handlers.go:159` | `rg -n 'recordAudit\(' internal/control/*.go` |
| A config update produces an audit row with non-empty `field_path`, proven by a unit test that fails against the current all-empty behavior. | VERIFIED | `internal/control/audit.go:154`, `internal/control/audit.go:176`, `internal/control/audit.go:200` | `TestRecordAuditFieldPathDerivedOnUpdate` |
| The chosen convention is documented and applied consistently across all `config_type` write paths. | VERIFIED | `internal/control/audit.go:16`; config write examples: `internal/control/config_admin_handlers.go:291`, `:551`, `:847`, `:1078`, `:1242`; admin config examples: `internal/control/admin_handlers.go:341`, `:1121`, `:1379` | `go test ./internal/control` |
| `config_audit_events.field_path` is non-empty for a live update if compose was used; otherwise handoff records live-pending with this task named as owner. | VERIFIED | Mirror receives derived `FieldPath` through `internal/control/audit.go:154`; live ClickHouse row below showed `field_path = 'Priority'` | `TestRecordAuditFieldPathDerivedOnUpdate`; live compose check below |

## Planning-Doc Coverage

| Planning item | Status | Evidence / owning task |
|---------------|--------|------------------------|
| P0 `GET /changes` config audit history endpoint | already existed | Task 31; this task only changes audit row fidelity |
| Config Audit Change `field_path` attribute | implemented | `internal/control/audit.go:16`, `internal/control/audit.go:154` |
| Config Audit Change old/new JSON alongside field path | already existed | Task 44; preserved by `internal/control/audit.go:152-165` |
| ClickHouse `config_audit_events.field_path String` column | already existed | `docs/planning/22-canonical-clickhouse-schema.md`; populated by `NewAuditStoreWithEvents` via `AuditRecord.FieldPath` |

Convention: call sites pass `*` for whole-resource mutations. `recordAudit` keeps `*` for create/delete/non-diffable
events and refines it to comma-separated changed field paths when both redacted old/new JSON payloads are objects.
This approximates the planning-doc per-field model without restructuring task 44's whole-object audit values.

## Verification

```sh
go test ./internal/control
make check
STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY=dev-admin-key-0001 docker compose up -d --build control
curl -fsS http://localhost:9090/readyz
PUT /api/v1/config/routing-rules/dev-default-route
SELECT ... FROM straw.config_audit_events ... FORMAT Vertical
```

Result:

- `go test ./internal/control`: passed.
- `make check`: passed (`go test ./...`; `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0`, 0
  issues).
- Postgres-backed tests: not run — no `postgres_*` files or migrations changed.
- Live compose verification: ran 2026-07-07 against the running compose stack after rebuilding `control`. A real
  `PUT /api/v1/config/routing-rules/dev-default-route` changed priority `1 -> 2`; ClickHouse row:
  `config_type = routing_rule`, `resource_id = dev-default-route`, `action = upsert`, `field_path = Priority`,
  `old_value_json` and `new_value_json` both populated. The temporary tenant admin key minted for this check was
  revoked before handoff.

## Reviewer Start Points

- `internal/control/audit.go`
- `internal/control/audit_test.go`
- `internal/control/config_admin_handlers.go`

## Remaining Work

- None. The task uses the existing whole-object audit model and adds no fake/stubbed backend.

## Blockers

- None.

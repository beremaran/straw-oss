# 31 - Config Change History API

Status: not started

## Objective

Implement the P0 `GET /changes` config audit-history endpoint with pagination over `config_audit_source`.

## Context (gap being closed)

The 2026-07-04 review found `docs/planning/26`'s P0 `GET /changes` endpoint is not wired. The backing table
(`config_audit_source`) and an unpaginated `postgresAuditStore.ListTenant` already exist from task 18; this task adds
pagination and the read-only HTTP surface. Audit rows are already redacted at write time (task 19), so this is a
read-only exposure task.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (`/changes` endpoint, shared list contract, RBAC)
- `docs/planning/25-dynamic-configuration.md` (config audit history intent)
- `docs/planning/21-state-and-storage.md` (`config_audit_source` as the audit source of truth)
- `docs/planning/27-security-controls.md` (no secret values in audit output)

## Prerequisites

- Task 18 completed (audit store).
- Task 20 completed (config-admin handler pattern).

## Out of Scope

- Do not implement the ClickHouse `config_audit_events` mirror (that is task 32).
- Do not implement config rollback (P1).
- Do not add write or mutation behavior to this endpoint.

## Expected Files

- Modify: `internal/control/postgres_audit_store.go` (add a paginated list honoring the shared list contract:
  `limit` default 50/max 200, `offset`, `created_at DESC` then `id ASC`).
- Modify: `internal/control/config_admin_handlers.go` (add the `ListChanges` handler).
- Modify: `cmd/control/main.go` (register `GET /api/v1/config/changes`).
- Test: `internal/control/config_admin_handlers_test.go`.

## Steps

- [ ] Read all required planning docs.
- [ ] Add a paginated audit list method scoped to a tenant with the shared list ordering/limits.
- [ ] Add the `ListChanges` handler with RBAC `tenant_admin`/`operator`/`viewer` (per the `docs/planning/26` row) and
      tenant scoping so it never returns another tenant's rows.
- [ ] Confirm the response carries no secret material (the stored rows are already redacted; assert it in a test).
- [ ] Register the route.
- [ ] Add tests for: pagination defaults and bounds; tenant isolation; role access; absence of secret values.
- [ ] Run the focused tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- `GET /api/v1/config/changes` returns the caller's tenant config audit history, paginated per the shared contract.
- The endpoint never returns another tenant's audit rows or any secret value.
- RBAC matches the `docs/planning/26` table.

## Handoff Notes

- Document the pagination behavior and the fields returned.
- Note that the ClickHouse audit mirror is owned by task 32.

## Stop Conditions

- Stop before adding rollback or mutation behavior.
- Stop if the endpoint would expose a field not permitted by `docs/planning/27`.
- Stop if a deferral would have no owning task file.

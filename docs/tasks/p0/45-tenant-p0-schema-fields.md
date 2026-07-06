# 45 - Tenant P0 Schema Fields

Status: not started

## Objective

Add the four canonical P0 tenant fields — `default_timeout_ms`, `max_timeout_ms`, `metadata_query_storage`,
`metadata_path_storage` — to the tenant schema, store, and admin API, and enforce them: request timeout defaulting
and clamping uses the tenant's values, and ClickHouse metadata sanitization of `target_url` follows the tenant's
query/path storage policy instead of the current hard-coded behavior.

## Context (gap being closed)

`docs/planning/26-config-management-api-surface.md` lists all four fields in the Tenant schema under "P0 Config
Resource Schemas" (lines ~124-141: "minimal canonical P0 shapes ... must not remove these fields"). Task
`docs/tasks/p0/29-tenant-lifecycle-and-status-enforcement.md` skipped them via an Out of Scope line calling them
"P1 tenant metadata-storage-policy fields" — the exact "assumed P1, not checked" failure the Gap Ownership rules in
`AGENTS.md` name. The 2026-07-06 handoff sweep (this task's provenance) verified the gap in current code:

- `internal/control/tenant_store.go:27-42` — `Tenant` struct has no timeout or metadata-storage fields.
- `migrations/postgres/0003_tenant_fields.sql` — adds name/ceiling/config_version only.
- `internal/control/request_metadata.go:244-258` — `sanitizeTargetURL` unconditionally drops the query and stores
  the full path; no tenant policy, no `hash` option (planning/21 says default is `hash` for path).
- Timeout defaulting/clamping is static-config only (`internal/config/config.go:80`,
  `internal/control/dispatcher.go:76,691`); no per-tenant override exists.

Flagged (as "P1 fields ... not added") in
`docs/agents/handoffs/29-tenant-lifecycle-and-status-enforcement.md`.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (Tenant schema, lines ~124-141; Shared Config API Contract)
- `docs/planning/21-state-and-storage.md` (metadata sanitization and `metadata_path_storage` semantics, lines
  ~95-116; defaults: query dropped unless policy allows, path default `hash`)
- `docs/planning/07-public-api-surface.md` (timeout validation rules, lines ~104-105)

## Prerequisites

- Task 29 completed (tenant lifecycle, store, and PUT/GET handlers exist).
- Task 32 completed (request metadata writer exists to enforce storage policy in).

## Out of Scope

- Do not build telemetry read APIs over the stored metadata (P1 tasks 11/12 own that).
- Do not add payload/header capture (P2).
- Do not change the static `control.request.*` config keys; tenant values layer on top of them (tenant
  `max_timeout_ms` may not exceed the static ceiling).

## Expected Files

- Add: `migrations/postgres/000X_tenant_p0_fields.sql` (four columns with planning-doc defaults:
  timeouts 60000/300000, `metadata_query_storage` `drop`, `metadata_path_storage` `hash`).
- Modify: `internal/control/tenant_store.go`, `internal/control/postgres_tenant_store.go` (fields + validation of
  enum values and timeout bounds).
- Modify: tenant PUT/GET handlers (`internal/control/admin_handlers.go` or wherever task 29 registered them) to
  accept/return the fields, `system_admin`/tenant-admin rules per planning/26.
- Modify: `internal/control/dispatcher.go` (or the REST request handler) so per-request timeout defaulting and
  rejection use the tenant's `default_timeout_ms`/`max_timeout_ms`, clamped by static config.
- Modify: `internal/control/request_metadata.go` so `sanitizeTargetURL` applies the tenant's
  `metadata_query_storage`/`metadata_path_storage` (`store | hash | drop`), with a stable hash for `hash`.
- Test: store round-trip and enum/bound validation; timeout defaulting and rejection per tenant values; one test
  per storage mode asserting stored `target_url` shape (query stored/hashed/dropped, path stored/hashed/dropped).

## Steps

- [ ] Read all required planning docs.
- [ ] Write the migration with planning-doc defaults.
- [ ] Extend `Tenant`, the Postgres store, and validation.
- [ ] Extend tenant PUT/GET handlers and their RBAC checks.
- [ ] Wire tenant timeouts into request validation in `cmd/control`'s dispatch path.
- [ ] Wire storage policy into `sanitizeTargetURL` via the tenant config snapshot in `cmd/control`'s metadata writer.
- [ ] Add the tests listed in Expected Files.
- [ ] Run focused tests, then `make check`.
- [ ] Update the stale "P1 fields" notes in `docs/tasks/p0/29-...md` and
      `docs/agents/handoffs/29-...md` to point here.
- [ ] Write a handoff note.

## Tests

- `go test ./internal/control`
- `make check` (Postgres-backed tests with `STRAW_TEST_POSTGRES_DSN` against `straw_test`)

## Acceptance Criteria

- `GET/PUT /api/v1/config/tenants/{id}` round-trips all four fields; invalid enum values and
  `default_timeout_ms > max_timeout_ms` are rejected with `invalid_request` (proven by handler tests).
- A request with no timeout gets the tenant's `default_timeout_ms`; a request above the tenant's `max_timeout_ms`
  is rejected (proven by tests).
- Stored `target_url` obeys each storage mode; with defaults, query is dropped and path is hashed (proven by
  tests; grep shows `sanitizeTargetURL` no longer ignores tenant policy).
- The "P1 fields" wording in task 29's file and handoff is replaced with a pointer to this task.

## Handoff Notes

- Record the hash function chosen for `hash` mode and where the tenant snapshot is read on the metadata path.
- Record how tenant timeouts compose with the static `control.request.*` ceilings.

## Stop Conditions

- Stop if planning docs conflict on defaults (26 vs 21 vs 24) — ask instead of picking.
- Stop if a deferral would have no owning task file.

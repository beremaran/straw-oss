# 04 - Postgres Schema

Status: not started

## Objective

Create the P0 Postgres schema for tenants, keys, workers, pools, routes, deny rules, injection policies, rate limits, quotas, config versions, and audit source records.

## Required Planning Docs

- `docs/planning/06-identity-roles-and-tenant-isolation.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/20-rate-limits-and-quotas.md`
- `docs/planning/21-state-and-storage.md`
- `docs/planning/24-static-configuration.md`
- `docs/planning/25-dynamic-configuration.md`
- `docs/planning/27-security-controls.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 01 completed.

## Out of Scope

- Do not implement config APIs.
- Do not implement Redis cache invalidation.
- Do not store API key secrets in plaintext.
- Do not add billing-grade quota reconciliation.

## Expected Files

- Create: `migrations/postgres/*.sql`
- Create or modify: schema test harness under the existing test pattern.
- Modify: `docker-compose.yml` only if needed to run local Postgres migrations.

## Steps

- [ ] Read all required planning docs.
- [ ] Write migrations for durable P0 config tables.
- [ ] Add constraints for tenant isolation, unique worker identity, route ordering, and config versioning.
- [ ] Add columns for audit source records without storing sensitive secret values.
- [ ] Add migration tests or a reproducible local migration command.
- [ ] Verify migrations apply to a clean local database.
- [ ] Verify migration re-run behavior is documented or guarded.
- [ ] Run focused migration checks.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Migration apply command chosen by the implementation.
- `go test ./...`
- `make check`

## Acceptance Criteria

- Clean Postgres can apply all migrations.
- Schema supports every P0 durable object named in the objective.
- Constraints prevent obvious tenant and worker identity collisions.
- Sensitive key material is not stored as plaintext secrets.

## Handoff Notes

- Include the exact migration command used.
- Note any schema choices that must be mirrored by application code.

## Stop Conditions

- Stop before adding application-level config APIs.
- Stop if the schema needs an undecided billing or P2 policy.

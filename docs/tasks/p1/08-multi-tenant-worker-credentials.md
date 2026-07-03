# 08 - Multi-Tenant Worker Credentials

Status: not started

## Objective

Add platform-scoped creation and validation of worker credentials that can serve multiple tenants.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md`
- `docs/planning/06-identity-roles-and-tenant-isolation.md`
- `docs/planning/11-worker-discovery-and-health.md`

## Prerequisites

- P0 task 18 completed.
- P0 task 20 completed.

## Out of Scope

- Do not let tenant-scoped keys create multi-tenant credentials.
- Do not add marketplace or provider billing behavior.
- Do not weaken worker registration signature validation.

## Expected Files

- Modify: worker credential stores and handlers.
- Modify: registration validation.
- Test: multi-tenant credential tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Extend worker credential create/update validation for platform-scoped `system_admin` multi-tenant scope.
- [ ] Persist allowed tenant/pool scopes as scoped objects.
- [ ] Enforce that tenant admins can still create only single-tenant credentials for their tenant.
- [ ] Validate worker registration capabilities against every allowed tenant/pool scope.
- [ ] Ensure routing never selects a worker for a tenant/pool outside its credential.
- [ ] Add tests for system_admin creation, tenant-admin rejection, cross-tenant routing isolation, and registration
      scope validation.
- [ ] Run focused worker credential tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused worker credential tests.
- `make check`

## Acceptance Criteria

- Multi-tenant credentials are platform-scoped only.
- Tenant-scoped credentials preserve P0 behavior.
- Routing and registration enforce every credential scope.

## Handoff Notes

- Document the stored scope shape and migration behavior.

## Stop Conditions

- Stop before adding public marketplace or billing behavior.
- Stop if a deferral would have no owning task file.

# 08 - Multi-Tenant Worker Credentials

Status: done

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

- [x] Read all required planning docs.
- [x] Extend worker credential create validation for platform-scoped `system_admin` multi-tenant scope. The required
      planning docs do not define a worker credential update endpoint.
- [x] Persist allowed tenant/pool scopes as scoped objects.
- [x] Enforce that tenant admins can still create only single-tenant credentials for their tenant.
- [x] Validate worker registration capabilities against every allowed tenant/pool scope.
- [x] Ensure routing never selects a worker for a tenant/pool outside its credential.
- [x] Add tests for system_admin creation, tenant-admin rejection, cross-tenant routing isolation, and registration
      scope validation.
- [x] Run focused worker credential tests.
- [x] Run `make check`.
- [x] Write a handoff note.

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

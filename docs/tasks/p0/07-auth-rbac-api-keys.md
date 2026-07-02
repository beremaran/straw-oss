# 07 - Auth, RBAC, and API Keys

Status: not started

## Objective

Implement API-key authentication, tenant resolution, RBAC, API key lifecycle, and revocation cache invalidation.

## Required Planning Docs

- `docs/planning/06-identity-roles-and-tenant-isolation.md`
- `docs/planning/07-public-api-surface.md`
- `docs/planning/25-dynamic-configuration.md`
- `docs/planning/26-config-management-api-surface.md`
- `docs/planning/27-security-controls.md`
- `docs/planning/30-testing-matrix.md`

## Prerequisites

- Task 04 completed.
- Task 05 completed.
- Task 06 completed.

## Out of Scope

- Do not implement OAuth or user sessions.
- Do not let tenant keys create tenants.
- Do not log API key secrets.
- Do not implement billing or marketplace workflows.

## Expected Files

- Create or modify: `internal/control`
- Create or modify: auth and key store packages under existing boundaries.
- Test: auth, RBAC, and revocation tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement API key hash verification.
- [ ] Resolve platform-scoped and tenant-scoped identities.
- [ ] Enforce platform and tenant role permissions.
- [ ] Add platform key lifecycle after bootstrap.
- [ ] Add tenant API key and worker credential lifecycle.
- [ ] Invalidate cached auth/config state on revocation.
- [ ] Add tests for platform key lifecycle, platform key cannot execute requests, tenant key cannot create tenants, revocation, actor audit source, and tenant isolation.
- [ ] Run focused auth tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused auth/RBAC tests.
- `go test ./...`
- `make check`

## Acceptance Criteria

- API keys authenticate without storing plaintext secrets.
- RBAC blocks cross-scope actions.
- Revocation takes effect through cache invalidation.
- Tests cover the auth rows in `docs/planning/30-testing-matrix.md`.

## Handoff Notes

- Document bootstrap behavior.
- List roles and allowed P0 actions.

## Stop Conditions

- Stop if a permission is not defined in planning docs.
- Stop before adding non-key authentication.

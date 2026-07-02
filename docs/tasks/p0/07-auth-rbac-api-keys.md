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
- Do not allow multi-tenant worker credentials: P0 creation forces `tenant_scope` to the caller's tenant and rejects
  `allowed_pools` entries referencing any other tenant (multi-tenant credentials are a P1 platform-scoped operation).

## Expected Files

- Create or modify: `internal/control`
- Create or modify: auth and key store packages under existing boundaries.
- Test: auth, RBAC, and revocation tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Implement API key hash verification.
- [ ] Resolve platform-scoped and tenant-scoped identities.
- [ ] Enforce platform and tenant role permissions.
- [ ] Add platform key lifecycle after bootstrap (`config_version` is 0 for platform keys — no tenant version applies).
- [ ] Add tenant API key and worker credential lifecycle; enforce the single-tenant worker-credential restriction.
- [ ] Enforce platform-managed quota writes: `PUT /tenants/{id}/quotas` requires `system_admin`; tenant keys retain read-only `/quotas` access.
- [ ] Invalidate cached auth/config state on revocation.
- [ ] Add tests for platform key lifecycle, platform key cannot execute requests, tenant key cannot create tenants, revocation, actor audit source, tenant isolation, quota write requires platform key, and worker-credential create rejects foreign tenant scope.
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

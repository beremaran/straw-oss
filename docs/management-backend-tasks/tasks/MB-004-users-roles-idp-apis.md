# MB-004: Users, Roles, And Identity Provider APIs

Status: done
Phase: 1
Depends on: MB-001, MB-002
Search tags: users, roles, identity-providers, `users:read`, `users:write`

## Objective

Expose management APIs for admin users, roles, permissions, and SSO provider configuration.

## Scope

- Add user list, detail, create, patch, and deactivate endpoints.
- Add role list, create, patch, and delete endpoints.
- Add identity-provider list, create, patch, and delete/disable endpoints.
- Protect all routes with `users:read` or `users:write`.
- Prevent deletion of built-in roles and prevent users from removing the last active owner.

## Repo Touchpoints

- `internal/server/admin/server.go`
- `internal/server/admin/handlers/*`
- `internal/server/dto/*`
- `internal/domain/*`
- `internal/infra/postgres/*_repo.go`
- `api/openapi.yaml`
- `docs/management-api.md`

## Implementation Tasks

- [x] Add DTOs with redaction for password hashes and provider secrets.
- [x] Add handlers and route registration.
- [x] Add repository methods needed by list/detail/update flows.
- [x] Emit structured audit events for mutating operations.
- [x] Add handler tests for permissions, validation, and protected-role behavior.

## Done Criteria

- [x] Users can be created, deactivated, assigned roles, and listed.
- [x] Roles can be managed except built-in role deletion.
- [x] Identity providers can be configured without exposing secrets in responses.
- [x] All mutating operations produce structured audit events.

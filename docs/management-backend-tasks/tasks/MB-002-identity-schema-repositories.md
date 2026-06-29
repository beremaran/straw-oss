# MB-002: Identity Schema And Repositories

Status: not-started
Phase: 1
Depends on: MB-001
Search tags: admin_users, admin_roles, admin_sessions, admin_identity_providers, repositories

## Objective

Persist admin users, roles, permissions, sessions, and identity providers.

## Scope

- Add migrations for `admin_users`, `admin_roles`, `admin_role_permissions`, `admin_user_roles`, `admin_sessions`, and `admin_identity_providers`.
- Seed built-in roles and permissions.
- Add domain types and Postgres repositories needed by auth, user management, SSO, and sessions.
- Store provider secrets as references or encrypted values, never plaintext JSON config.

## Repo Touchpoints

- `internal/infra/postgres/migrations/*.sql`
- `internal/infra/postgres/migrations.go`
- `internal/domain/*`
- `internal/infra/postgres/*_repo.go`
- `internal/infra/postgres/*_repo_test.go`

## Implementation Tasks

- [ ] Create the identity migration after the current latest migration.
- [ ] Add built-in role seed data for Owner, Operator, Security auditor, Finance, and Read only.
- [ ] Add domain models for user, role, permission, session, and identity provider.
- [ ] Add repositories for CRUD and lookup paths used by auth.
- [ ] Cover repository create, update, list, and lookup behavior with tests.

## Done Criteria

- [ ] Migrations apply cleanly on an empty database.
- [ ] Built-in roles exist and have the permissions from the spec.
- [ ] Repositories can resolve a user's effective permissions.
- [ ] Session repository stores only refresh-token hashes.

# MB-003: Local Sessions And Bootstrap Owner

Status: done
Phase: 1
Depends on: MB-001, MB-002
Search tags: login, refresh, logout, bootstrap, password hash, session rotation

## Objective

Add local email/password login, access-token issuance, refresh-token rotation, logout, current-user lookup, and first-owner bootstrap.

## Scope

- Implement `/management/auth/login`, `/management/auth/refresh`, `/management/auth/logout`, and `/management/auth/me`.
- Implement `/management/users/bootstrap` for the first owner.
- Hash local passwords with the smallest acceptable secure option already available or a vetted minimal dependency if needed.
- Rotate refresh tokens and revoke the session family on reuse.
- Warn at startup when no active owner exists.

## Repo Touchpoints

- `internal/server/admin/server.go`
- `internal/server/admin/handlers/*`
- `internal/server/admin/middleware/auth.go`
- `internal/config/config.go`
- `internal/service/auth/*`
- `internal/infra/postgres/*session*_repo.go`
- `internal/server/admin/handlers/*_test.go`

## Implementation Tasks

- [x] Add request and response DTOs for auth endpoints.
- [x] Add password verification and access-token signing.
- [x] Add refresh-token creation, hashing, rotation, expiry, and reuse detection.
- [x] Add bootstrap route guarded by the legacy management token and owner-existence check.
- [x] Add tests for login failure, successful login, refresh rotation, logout revocation, and bootstrap disablement.

## Done Criteria

- [x] Admin users can authenticate without the legacy management token.
- [x] Access tokens expire and include enough actor/session data for RBAC.
- [x] Refresh tokens rotate and old-token reuse revokes the session family.
- [x] Bootstrap is unavailable after an active owner exists.

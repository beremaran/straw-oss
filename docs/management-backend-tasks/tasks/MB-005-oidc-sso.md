# MB-005: OIDC SSO Login Flow

Status: done
Phase: 1
Depends on: MB-002, MB-003, MB-004
Search tags: sso, oidc, authorization code, callback, jwks, role claim

## Objective

Allow enabled OIDC providers to start and complete admin login.

## Scope

- Implement `/management/auth/sso/{provider}/start`.
- Implement `/management/auth/sso/{provider}/callback`.
- Validate provider state, issuer, ID token signature, nonce, audience, and expiry.
- Map email/profile claims to admin users.
- Map configured role claim or default role to permissions.

## Repo Touchpoints

- `internal/server/admin/server.go`
- `internal/server/admin/handlers/*`
- `internal/service/auth/*`
- `internal/infra/postgres/*identity*_repo.go`
- `internal/config/config.go`
- `internal/server/admin/handlers/*_test.go`

## Implementation Tasks

- [x] Add provider lookup and enabled checks.
- [x] Add start URL generation with state and nonce.
- [x] Add callback token exchange and ID-token validation.
- [x] Add user upsert or link behavior for trusted provider emails.
- [x] Issue the same access and refresh tokens as local login.

## Done Criteria

- [x] Disabled or unknown providers cannot start login.
- [x] Callback rejects invalid state, issuer, audience, expiry, and signature.
- [x] Successful SSO login creates or updates the admin user and session.
- [x] Provider secrets are never returned through management read APIs.

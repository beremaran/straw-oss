# MB-005: OIDC SSO Login Flow

Status: not-started
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

- [ ] Add provider lookup and enabled checks.
- [ ] Add start URL generation with state and nonce.
- [ ] Add callback token exchange and ID-token validation.
- [ ] Add user upsert or link behavior for trusted provider emails.
- [ ] Issue the same access and refresh tokens as local login.

## Done Criteria

- [ ] Disabled or unknown providers cannot start login.
- [ ] Callback rejects invalid state, issuer, audience, expiry, and signature.
- [ ] Successful SSO login creates or updates the admin user and session.
- [ ] Provider secrets are never returned through management read APIs.

# MB-001: Auth And RBAC Compatibility Foundation

Status: done
Phase: 1
Depends on: none
Search tags: auth, rbac, legacy token, permissions, actor context, `MANAGEMENT_LEGACY_TOKEN_ENABLED`

## Objective

Replace the management-only static-token assumption with a shared authentication and authorization foundation while preserving the current `MANAGEMENT_API_KEY` path.

## Scope

- Add management actor context containing actor type, actor ID, display name, session ID, and permissions.
- Preserve legacy bearer-token auth as synthetic `system:legacy-admin` with all permissions while enabled.
- Add permission checks for `/management/*` handlers without changing existing public route behavior.
- Add config for disabling legacy token compatibility later.
- Keep response errors in the existing `{error, code, details}` shape where handlers already support it.

## Repo Touchpoints

- `internal/server/admin/server.go`
- `internal/server/admin/middleware/auth.go`
- `internal/server/admin/middleware/audit.go`
- `internal/config/config.go`
- `internal/server/helper/helper.go`
- `internal/server/admin/middleware/*_test.go`

## Implementation Tasks

- [x] Define actor and permission context helpers.
- [x] Convert management auth middleware to resolve either legacy token or user session.
- [x] Add permission middleware usable per route.
- [x] Register existing routes with equivalent permissions from the spec.
- [x] Keep legacy token tests passing and add denial tests for missing permissions.

## Done Criteria

- [x] Existing management routes still accept the current legacy bearer token by default.
- [x] A request with no valid management auth still returns `401`.
- [x] A request with a valid actor but missing permission returns `403`.
- [x] Handlers and audit logging can read actor metadata from request context.

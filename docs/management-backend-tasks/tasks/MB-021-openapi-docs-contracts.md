# MB-021: OpenAPI, Docs, And Contract Test Sweep

Status: not-started
Phase: cross-cutting
Depends on: MB-003 through MB-020
Search tags: openapi, management-api, contract tests, response schemas, compatibility

## Objective

Keep the public Management API contract accurate and compatibility-tested as backend support lands.

## Scope

- Add every new route to `api/openapi.yaml`.
- Update `docs/management-api.md`.
- Add or update response schemas used by contract tests.
- Confirm backward-compatible route behavior from the spec.
- Keep routes under `/management/*` and behind the new auth/RBAC middleware.

## Repo Touchpoints

- `api/openapi.yaml`
- `docs/management-api.md`
- `test/contract/*`
- `test/contract/schemas/*.json`
- `internal/server/admin/server.go`
- `internal/server/admin/handlers/*_test.go`

## Implementation Tasks

- [ ] Add OpenAPI paths, request schemas, response schemas, and error responses for new routes.
- [ ] Update Management API docs with auth modes and new capabilities.
- [ ] Add contract schemas where response shape needs drift protection.
- [ ] Verify existing `POST /management/api-keys`, `GET /management/api-keys`, `DELETE /management/api-keys/{id}`, `POST /management/endpoints/{id}/drain`, `POST /management/fingerprints`, and billing estimate behavior.
- [ ] Run contract tests and fix drift.

## Done Criteria

- [ ] OpenAPI includes all implemented Management API routes.
- [ ] Docs match implemented behavior.
- [ ] Contract tests pass.
- [ ] Existing route compatibility is explicitly tested.

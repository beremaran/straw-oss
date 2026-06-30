# MB-022: Final Acceptance And Compatibility Pass

Status: done
Phase: cross-cutting
Depends on: MB-021
Search tags: final acceptance, regression, legacy routes, management backend spec

## Objective

Prove the Management Backend Specification is fully implemented and existing behavior still works.

## Scope

- Run the spec acceptance checklist end to end.
- Run unit, integration, security, and contract tests.
- Verify legacy-token compatibility and user-session auth.
- Verify Management UI backend dependency items are all supported.
- Update tracker statuses and close any missed tasks.

## Repo Touchpoints

- `docs/management-backend-spec.md`
- `docs/management-ui-spec.md`
- `docs/management-backend-tasks/tracker.md`
- `test/integration/*`
- `test/security/*`
- `test/contract/*`
- `Makefile`

## Implementation Tasks

- [x] Convert every acceptance checklist item in the spec into a concrete verification note.
- [x] Run `go test ./...`.
- [x] Run any documented security, integration, load, and contract checks that apply.
- [x] Verify no new management route bypasses auth/RBAC.
- [x] Verify no read API returns stored secrets.
- [x] Update tracker and task statuses to reflect completed work.

## Verification Notes

- Specification phases MB-001 through MB-021 are marked `done` in the task tracker.
- `api/openapi.yaml` includes all implemented management routes; `TestOpenAPIIncludesManagementRoutes` covers route drift.
- Legacy compatibility routes remain covered: API key create/list/delete, endpoint drain, fingerprint upsert, and billing estimate schemas.
- `TestServer_ManagementRoutesRequireAuth` verifies protected management routes reject unauthenticated requests before handlers run.
- Secret-return risk is covered by redacted notification channel responses and API key/token DTOs that expose raw secrets only on create/rotate responses.
- Management UI backend dependencies listed in the spec are represented in OpenAPI and docs.
- Final commands run successfully: `go test ./...`, `make docs`, and `make build lint test`.

## Done Criteria

- [x] All checklist items in `docs/management-backend-spec.md` pass.
- [x] Existing Management API routes continue to pass their current tests.
- [x] New Management UI backend dependencies are implemented.
- [x] Any remaining deferred work is documented as a new explicit task.

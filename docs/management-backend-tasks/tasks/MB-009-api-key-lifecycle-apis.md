# MB-009: API Key Lifecycle APIs

Status: done
Phase: 2
Depends on: MB-008
Search tags: api keys, detail, update, rotate, reactivate, revoke, `api_keys:rotate`, raw_key

## Objective

Add full API key lifecycle management under a stable logical key ID.

## Scope

- Add `GET /management/api-keys/{id}`.
- Add `PATCH /management/api-keys/{id}`.
- Add `POST /management/api-keys/{id}/rotate`.
- Add `POST /management/api-keys/{id}/reactivate`.
- Add `POST /management/api-keys/{id}/revoke`.
- Keep `DELETE /management/api-keys/{id}` as a revoke compatibility alias.

## Repo Touchpoints

- `internal/server/admin/handlers/api_key.go`
- `internal/server/admin/server.go`
- `internal/server/dto/api_key.go`
- `internal/infra/postgres/api_key_repo.go`
- `internal/domain/api_key.go`
- `api/openapi.yaml`
- `docs/management-api.md`

## Implementation Tasks

- [x] Add DTOs for detail, update, rotation request, and rotation response.
- [x] Validate optional fields, scope syntax, rate limits, and expiration changes.
- [x] Implement rotate with one-time `raw_key`, immediate revoke, or grace-period behavior.
- [x] Implement reactivate only for non-expired logical keys.
- [x] Emit structured audit events without storing `raw_key`.

## Done Criteria

- [x] Key detail returns token history metadata but no hashes or raw tokens.
- [x] Update can change name, scopes, rate limit override, expiration, and active state.
- [x] Rotation returns a raw secret once and updates previous token statuses correctly.
- [x] Revoke disables the logical key and all token secrets.

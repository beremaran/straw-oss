# MB-010: Fingerprint Detail And Protected Delete

Status: not-started
Phase: 2
Depends on: MB-001, MB-006
Search tags: fingerprints, delete, `fingerprints:delete`, routing rule dependency, broadcast

## Objective

Add fingerprint preset detail and safe deletion with routing-rule dependency protection.

## Scope

- Add `GET /management/fingerprints/{id}`.
- Add `DELETE /management/fingerprints/{id}`.
- Check active routing rules for references in `fingerprint_preset` and `fingerprint_ab_test.variants`.
- Return `409` with referencing rule IDs and names when deletion is unsafe.
- Support owner-only force behavior from the spec.
- Broadcast preset changes unless `broadcast=false`.

## Repo Touchpoints

- `internal/server/admin/handlers/fingerprints.go`
- `internal/server/admin/server.go`
- `internal/infra/postgres/fingerprint_repo.go`
- `internal/infra/postgres/routing_rule_repo.go`
- `internal/domain/fingerprint.go`
- `internal/domain/routing_rule.go`
- `api/openapi.yaml`
- `docs/management-api.md`

## Implementation Tasks

- [ ] Add detail handler and route.
- [ ] Add repository query for active routing rules referencing a preset.
- [ ] Add delete handler with conflict response and optional force path.
- [ ] Publish fingerprint broadcast after successful delete unless disabled.
- [ ] Add audit event with deleted preset metadata and redacted config.

## Done Criteria

- [ ] Referenced presets cannot be deleted without explicit allowed force behavior.
- [ ] Force delete is limited to Owner and handles affected rules as the spec requires.
- [ ] Successful delete returns `{id, deleted, broadcast_requested}`.
- [ ] Existing list and upsert behavior remains compatible.

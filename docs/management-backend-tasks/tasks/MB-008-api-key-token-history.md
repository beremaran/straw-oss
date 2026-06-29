# MB-008: API Key Token History And Auth Lookup Migration

Status: not-started
Phase: 2
Depends on: MB-001
Search tags: api_key_tokens, token hash, active, grace, revoked, expired, auth lookup

## Objective

Split API key token secrets from logical API key metadata and migrate authentication to token history.

## Scope

- Add `api_key_tokens` migration.
- Copy existing `api_keys.token_hash` values into `api_key_tokens` as `active`.
- Update client API authentication lookup to use active or grace token rows.
- Keep `api_keys.token_hash` temporarily for rollback compatibility.
- Add token status and expiration handling.

## Repo Touchpoints

- `internal/infra/postgres/migrations/*.sql`
- `internal/infra/postgres/api_key_repo.go`
- `internal/domain/api_key.go`
- `internal/service/auth/*`
- `internal/server/middleware/auth.go`
- `test/security/auth_test.go`

## Implementation Tasks

- [ ] Add migration with indexes from the spec.
- [ ] Add domain and repository methods for token creation, lookup, status changes, and history list.
- [ ] Update API-key auth middleware to resolve by `api_key_tokens`.
- [ ] Treat expired tokens as rejected even before status cleanup runs.
- [ ] Add repository and security tests for active, grace, revoked, and expired tokens.

## Done Criteria

- [ ] Existing API keys still authenticate after migration.
- [ ] New token lookup ignores revoked and expired tokens.
- [ ] Grace tokens authenticate only until `expires_at`.
- [ ] Logical API key ID remains stable across token changes.

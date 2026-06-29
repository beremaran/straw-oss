# MB-006: Structured Audit Foundation And Redaction

Status: not-started
Phase: 1
Depends on: MB-001
Search tags: audit, structured audit, redaction, actor, request_id, trace_id

## Objective

Add structured management audit events and make request audit safe enough for viewer APIs.

## Scope

- Add actor fields to `admin_audit_log`.
- Add `management_audit_events`.
- Add an audit event writer reusable by handlers.
- Redact secrets in request audit and structured audit payloads.
- Update existing mutating management handlers to emit structured audit events.

## Repo Touchpoints

- `internal/infra/postgres/migrations/*.sql`
- `internal/server/admin/middleware/audit.go`
- `internal/server/admin/handlers/*.go`
- `internal/infra/postgres/*audit*_repo.go`
- `internal/domain/*audit*.go`
- `internal/server/admin/middleware/audit_test.go`

## Implementation Tasks

- [ ] Create migration for request-audit actor fields and `management_audit_events`.
- [ ] Add audit domain model and repository.
- [ ] Add redaction helper for `raw_key`, `token`, `password`, `client_secret`, `Authorization`, and cookie-like fields.
- [ ] Wire request ID, trace ID, IP, user agent, and actor metadata into audit records.
- [ ] Emit structured events from current API key, routing rule, endpoint drain, fingerprint, and cache mutations.

## Done Criteria

- [ ] Mutating management requests write actor-aware request audit rows.
- [ ] Structured events include action, entity type, entity ID, old value, and new value where available.
- [ ] Secret values are redacted before storage.
- [ ] Existing audit middleware behavior remains non-blocking.

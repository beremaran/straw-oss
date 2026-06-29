# MB-006: Structured Audit Foundation And Redaction

Status: done
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

- [x] Create migration for request-audit actor fields and `management_audit_events`.
- [x] Add audit domain model and repository.
- [x] Add redaction helper for `raw_key`, `token`, `password`, `client_secret`, `Authorization`, and cookie-like fields.
- [x] Wire request ID, trace ID, IP, user agent, and actor metadata into audit records.
- [x] Emit structured events from current API key, routing rule, endpoint drain, fingerprint, and cache mutations.

## Done Criteria

- [x] Mutating management requests write actor-aware request audit rows.
- [x] Structured events include action, entity type, entity ID, old value, and new value where available.
- [x] Secret values are redacted before storage.
- [x] Existing audit middleware behavior remains non-blocking.

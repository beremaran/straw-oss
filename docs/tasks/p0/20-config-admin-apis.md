# 20 - Config Admin APIs for Routing, Deny, Injection, Fingerprint

Status: done

## Objective

Implement the missing P0 config-management HTTP surface for routing rules, deny rules, injection policies, and
read-only fingerprint profiles, backed by the Postgres stores from task 19.

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md`
- `docs/planning/06-identity-roles-and-tenant-isolation.md`
- `docs/planning/07-public-api-surface.md`
- `docs/planning/10-routing-model.md`
- `docs/planning/27-security-controls.md`

## Prerequisites

- Task 07 completed.
- Task 19 completed.

## Out of Scope

- Do not implement P1 rollback.
- Do not implement tenant-authored fingerprint profiles.
- Do not implement P2 payload-capture policy APIs.
- Do not implement request dispatch (task 24).

## Expected Files

- Create or modify: `internal/control` config/admin handlers.
- Modify: `cmd/control/main.go`
- Test: focused config API handler tests.

## Steps

- [x] Read all required planning docs.
- [x] Add `GET/POST/PUT/DELETE /api/v1/config/routing-rules` handlers with tenant scoping, role checks, pagination,
      soft delete, stable ID behavior, and `expected_config_version` conflict handling.
- [x] Add `GET/POST/PUT/DELETE /api/v1/config/deny-rules` handlers with normalization inputs needed by Section 27 and
      tenant-admin-only writes.
- [x] Add `GET/POST/PUT/DELETE /api/v1/config/injection-policies` handlers with header safety validation, sensitive
      header role restrictions, operation count bounds, and secret audit redaction.
- [x] Add read-only `GET /api/v1/config/fingerprint-profiles` for built-in P0 profiles.
- [x] Ensure every successful config write increments tenant config version, records actor audit source, and calls the
      injected invalidation publisher interface after commit.
- [x] Wire the handlers into `cmd/control/main.go` using the existing `Authenticator`/RBAC patterns.
- [x] Add tests for role access, tenant isolation, pagination defaults, conflict responses, soft deletion, fingerprint
      read-only behavior, injection safety, deny-rule validation, and invalidation publisher calls.
- [x] Run focused config API tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- Focused config API handler tests.
- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- P0 routing, deny, and injection config APIs exist under `/api/v1/config` and are backed by Postgres stores.
- Fingerprint profiles are listable but not writable in P0.
- Config version conflicts return HTTP 409 `conflict` with the current version in details.
- Successful writes publish invalidation through the interface, even if the concrete Redis publisher lands in task 21.

## Handoff Notes

- Document every endpoint added and the roles allowed to call it.
- Note that Redis-backed invalidation implementation is deferred to `docs/tasks/p0/21-redis-wiring-and-config-invalidation.md`.

## Stop Conditions

- Stop before adding rollback or tenant-authored fingerprint-profile writes.
- Stop if a requested endpoint is not in Section 26's P0 table.
- Stop if a deferral would have no owning task file.

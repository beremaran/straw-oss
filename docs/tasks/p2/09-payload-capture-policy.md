# 09 - Payload Capture Policy

Status: not started

## Objective

Add the tenant payload-capture policy schema and config API.

## Required Planning Docs

- `docs/planning/19-payload-capture-p2.md`
- `docs/planning/26-config-management-api-surface.md`
- `docs/planning/07-public-api-surface.md`
- `docs/planning/02-phase-boundaries.md`

## Prerequisites

- P0 task 20 completed.

## Out of Scope

- Do not implement the capture engine.
- Do not add live traffic mutation.
- Do not enable capture by default.

## Expected Files

- Create or modify: payload-capture policy schema/store/handlers.
- Test: payload-capture policy API tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the `/api/v1/config/payload-capture` schema because Section 26 defers it until implementation starts.
- [ ] Implement `GET /api/v1/config/payload-capture` for tenant_admin, operator, and viewer.
- [ ] Implement `PUT /api/v1/config/payload-capture` for tenant_admin.
- [ ] Keep policy disabled by default.
- [ ] Extend request validation so `capture_hint` accepts only values allowed by the tenant policy.
- [ ] Add tests for schema validation, roles, disabled default, capture_hint authorization, conflict handling, and audit
      redaction.
- [ ] Run focused payload-capture policy tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused payload-capture policy API tests.
- `make check`

## Acceptance Criteria

- Payload capture policy is explicit, tenant-scoped, and disabled by default.
- `capture_hint` values beyond `none` are accepted only when policy allows them.
- No payload bytes are captured by this task.

## Handoff Notes

- Document the schema created from the previously deferred Section 26 row.

## Stop Conditions

- Stop before adding capture engine behavior.
- Stop if a deferral would have no owning task file.

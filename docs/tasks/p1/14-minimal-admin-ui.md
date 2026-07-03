# 14 - Minimal Admin UI

Status: not started

## Objective

Build a minimal read-mostly admin and observability UI over existing admin, config, and telemetry APIs.

## Required Planning Docs

- `docs/planning/02-phase-boundaries.md`
- `docs/planning/07-public-api-surface.md`
- `docs/planning/26-config-management-api-surface.md`

## Prerequisites

- Task 12 completed.
- P0 task 20 completed.

## Out of Scope

- Do not replace the API as source of truth.
- Do not add user/password auth.
- Do not implement marketplace or billing UI.

## Expected Files

- Create: minimal admin UI package/app according to repo convention.
- Test: UI smoke tests.

## Steps

- [ ] Read all required planning docs.
- [ ] Define the minimal UI routes/views before implementation.
- [ ] Add API-key based session/config for local operator use.
- [ ] Add read views for requests, workers, audit, tenants, routes, deny rules, and injection policies.
- [ ] Add narrowly scoped admin actions only where existing APIs already authorize them.
- [ ] Ensure UI hides fields tenant-facing APIs omit.
- [ ] Add smoke tests for navigation, auth failure, worker view, request lookup, and config read views.
- [ ] Run focused UI tests.
- [ ] Run `make check`.
- [ ] Write a handoff note.

## Tests

- Focused UI smoke tests.
- `make check`

## Acceptance Criteria

- The first screen is a usable admin surface, not a landing page.
- UI actions map only to existing APIs.
- Tenant-facing redaction rules are preserved.

## Handoff Notes

- List views and API endpoints consumed.

## Stop Conditions

- Stop before adding new backend capabilities to satisfy UI wishes.
- Stop if a deferral would have no owning task file.

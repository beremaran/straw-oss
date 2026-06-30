# MU-018: Final First-Release Acceptance Pass

Status: done
Phase: cross-cutting
Depends on: MU-015, MU-016, MU-017
Search tags: acceptance checklist, route coverage, first release, unsupported backend actions, final verification

## Objective

Verify the implemented Management UI satisfies the full first-release acceptance checklist in `docs/management-ui-spec.md`.

## Scope

- Walk every first-release route and workflow.
- Verify every registered Management API route has a visible UI surface or documented system-only/backend-gap reason.
- Verify unsupported backend actions are not exposed as enabled controls.
- Verify global states are consistent: loading, empty, unauthorized, network error, server error, validation error, mutation success, partial failure, stale data, and destructive pending.
- Verify destructive workflows require confirmation: API key revoke, endpoint drain, fingerprint broadcast, routing-rule delete/deactivate, and cache clear.
- Verify secret handling for management token and raw API keys.
- Verify final docs and tracker state.

## Repo Touchpoints

- `docs/management-ui-spec.md`
- `docs/management-ui-tasks/*`
- `web/management/*`
- `api/openapi.yaml`
- `docs/management-api.md`

## Implementation Tasks

- [x] Execute the first-release acceptance checklist from the UI spec.
- [x] Compare implemented API calls against `api/openapi.yaml` and `docs/management-api.md`.
- [x] Run the frontend build and tests.
- [x] Run any backend checks needed to prove integration was not broken.
- [x] Update `docs/management-ui-tasks/tracker.md` and task statuses.

## Done Criteria

- [x] Every item in the UI spec First-Release Acceptance Checklist is satisfied or explicitly blocked.
- [x] Blocked items name the missing backend/API dependency and link to the relevant task when available.
- [x] Build and test commands pass.
- [x] Tracker shows no stale in-progress work.

## Validation

2026-06-30:

- `npm run test` passed: 91 tests.
- `npm run lint` passed.
- `npm run build` passed.
- `go test -race -shuffle=on ./...` passed.
- `make lint` passed.
- Browser smoke passed against a mock Management API using backend-style paginated `data` list responses: sign-in, Overview, API Keys, and 390px mobile Overview.

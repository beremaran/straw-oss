# MU-014: System Diagnostics, Capability Detection, And Backend Gap Display

Status: done
Phase: 3
Depends on: MU-004
Search tags: `/system`, healthz, capabilities, backend gaps, docs links, sign out, secrets

## Objective

Build the System page for connection details, health, detected capabilities, documentation links, and known backend gaps.

## Scope

- Show Management API base URL and sign-out action.
- Call `GET /healthz` and show result plus response time.
- Detect backend capabilities for cache controls, fingerprints, usage, and other first-release surfaces based on successful/failed API calls.
- Link to Management API docs, OpenAPI reference, and architecture docs.
- List known backend gaps from the UI spec.
- Never show secrets.

## Repo Touchpoints

- `web/management/src/routes/system*`
- `web/management/src/components/system/*`
- `web/management/src/api/health*`
- `docs/management-api.md`
- `api/openapi.yaml`
- `docs/architecture.md`

## Implementation Tasks

- [x] Build health check panel with manual refresh.
- [x] Add capability detection from actual API results, not hardcoded assumptions.
- [x] Add docs links using repository paths or hosted docs paths chosen by the app.
- [x] Add backend gap list for unsupported first-release actions.
- [x] Audit the page to ensure no token or raw API key can render.

## Done Criteria

- [x] `/system` gives operators connection, health, capabilities, docs, and backend-gap context.
- [x] Capability labels update when a route is unavailable.
- [x] Secrets never appear on the page.
- [x] Unsupported features are documented as gaps, not enabled controls.


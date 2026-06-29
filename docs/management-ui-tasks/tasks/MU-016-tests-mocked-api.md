# MU-016: Tests, Mocked Management API Coverage, And Route Regression Checks

Status: not-started
Phase: 4
Depends on: MU-002 through MU-014
Search tags: tests, mocked API, e2e, route coverage, error states, validation, regression

## Objective

Add the smallest useful automated checks that prove the UI routes, API client behavior, validation, and destructive workflows keep working.

## Scope

- Add tests using the frontend stack selected in MU-001.
- Mock Management API responses for success, `401`, network error, server error, validation error, empty state, and partial failure.
- Cover auth header behavior, sign-in, protected route redirects, and sign-out.
- Cover representative domain workflows: create API key raw-key modal, revoke confirmation, rule version conflict, endpoint drain, fingerprint broadcast, usage date validation, and cache clear `CLEAR ALL`.
- Add route regression checks for every first-release route.

## Repo Touchpoints

- `web/management/src/**/*.test.*`
- `web/management/test*`
- frontend package scripts

## Implementation Tasks

- [ ] Add API client tests for auth injection and error normalization.
- [ ] Add validation tests for tags, scopes, durations, dates, integers, and JSON object config.
- [ ] Add route smoke tests with mocked data.
- [ ] Add focused workflow tests for high-risk destructive and secret-handling flows.
- [ ] Wire tests into the documented frontend command.

## Done Criteria

- [ ] Tests fail if auth is sent to `/healthz` or missing from `/management/*`.
- [ ] Tests fail if raw API keys can be dismissed before acknowledgement.
- [ ] Tests fail if cache clear `*` bypasses `CLEAR ALL`.
- [ ] Every first-release route has at least a mocked render or navigation check.


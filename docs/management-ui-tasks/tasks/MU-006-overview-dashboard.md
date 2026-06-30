# MU-006: Overview Dashboard And Operational Attention Panels

Status: done
Phase: 2
Depends on: MU-004, MU-005
Search tags: `/overview`, dashboard, endpoint health, usage, billing, cache, routing attention, partial failure

## Objective

Build the Overview page from existing Management API endpoints without adding a backend aggregate endpoint.

## Scope

- Load endpoint health, routing rules, API keys, usage summary, billing estimate, cache stats, and health as independent panels.
- Show cards for endpoint states, routing rules, API keys, seven-day usage, month-to-date billing, and cache status.
- Add Endpoint pressure table with endpoint ID, state, active tasks, tags, last seen, and drain action.
- Add Recent usage chart or compact visual with daily requests and bytes.
- Add Routing attention list for duplicate priorities, inactive rules, missing required tags, missing fingerprint presets, missing endpoint pool matches, insecure TLS, and disabled filters.
- Support partial-failure warning when some panels fail.

## Repo Touchpoints

- `web/management/src/routes/overview*`
- `web/management/src/components/dashboard/*`
- `web/management/src/api/*`

## Implementation Tasks

- [x] Fetch all Overview panel data through shared API helpers.
- [x] Calculate endpoint state counts and active task totals.
- [x] Calculate active/inactive API key and routing-rule totals.
- [x] Detect routing attention conditions client-side.
- [x] Keep successful panels visible when another panel fails.

## Done Criteria

- [x] `/overview` matches the spec cards and main sections.
- [x] Auth failure across aggregate requests returns the user to sign-in.
- [x] Partial backend failure shows successful data plus a retryable warning.
- [x] No unsupported backend action is implied by the Overview page.


# MU-004: App Shell, Routes, Navigation, And Global Data States

Status: done
Phase: 1
Depends on: MU-003
Search tags: app shell, routes, navigation, top bar, refresh, loading, empty, errors, stale data

## Objective

Add the console shell, first-release routes, navigation, refresh behavior, and shared global states.

## Scope

- Add routes for `/overview`, `/api-keys`, `/routing-rules`, `/routing-rules/new`, `/routing-rules/:id`, `/endpoints`, `/fingerprints`, `/usage`, `/cache`, and `/system`.
- Implement left navigation on desktop and collapsible or drawer navigation on smaller screens.
- Add top bar with environment label, base URL, connection status, refresh button, and sign-out menu.
- Add page headers with one primary action at the right when a page supports creation or mutation.
- Show "Last refreshed" timestamps on data pages.
- Keep previous data visible during refresh when possible.
- Standardize unauthorized, network error, server error, empty, loading, stale-data, mutation success, and partial-failure surfaces.

## Repo Touchpoints

- `web/management/src/routes/*`
- `web/management/src/components/shell/*`
- `web/management/src/components/state/*`
- `web/management/src/state/*`

## Implementation Tasks

- [ ] Create authenticated route guards and default post-login redirect to `/overview`.
- [ ] Add shell navigation labels exactly matching the spec.
- [ ] Add refresh actions and last-refreshed display to route-level data.
- [ ] Add skeleton rows/cards matching final layout shapes.
- [ ] Route `401` from authenticated pages back to sign-in with "Session token rejected".

## Done Criteria

- [ ] All first-release routes exist and are protected behind connection details.
- [ ] Navigation works by mouse and keyboard.
- [ ] Global state components are reused across pages instead of reimplemented per page.
- [ ] A failed panel can show an inline retry without blanking successful panels.


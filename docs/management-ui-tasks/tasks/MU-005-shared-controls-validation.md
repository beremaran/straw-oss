# MU-005: Shared Controls, Validation, And Mutation Workflows

Status: done
Phase: 1
Depends on: MU-004
Search tags: validation, tags, scopes, durations, dates, JSON, confirmation dialogs, partial failure, toasts

## Objective

Build the small set of shared controls and validators needed by the domain pages.

## Scope

- Add validation helpers for tags, scopes, Go duration strings, positive integers, non-negative integers, `YYYY-MM-DD` dates, and JSON objects.
- Add shared chip editors, token inputs, date range controls, number inputs, copy buttons, download buttons, and JSON editor wrapper.
- Add destructive confirmation dialogs for revoke, deactivate/delete, drain, broadcast, and cache clear.
- Add toast and inline alert primitives for mutation success, backend errors, and partial failures.
- Ensure field-level validation keeps form data and focuses the first invalid field.

## Repo Touchpoints

- `web/management/src/components/forms/*`
- `web/management/src/components/dialogs/*`
- `web/management/src/components/feedback/*`
- `web/management/src/validation/*`

## Implementation Tasks

- [x] Implement routing tag parsing for `key:value` and `key=value`, normalized to `key:value`.
- [x] Warn, but allow, rarely useful or future-facing values where the spec allows them.
- [x] Reject natural-language durations such as `30 seconds`.
- [x] Validate fingerprint config JSON as an object.
- [x] Add confirmation dialog behavior that disables repeated clicks while pending.

## Done Criteria

- [x] Domain pages can reuse one validator for each spec validation rule.
- [x] Destructive dialogs name the affected object and backend effect.
- [x] Partial failures show completed and failed items, with retry for failed items only.
- [x] Icon-only actions have labels/tooltips available to assistive technology.


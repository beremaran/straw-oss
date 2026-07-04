# 27 - Admin Request Cancellation Pipeline

Status: done

## Objective

Make `POST /api/v1/admin/requests/{request_id}/cancel` a working P0 endpoint that cancels an in-flight request end to
end, backed by an in-flight request registry and a real cancel dispatch to the executor.

## Context (gap being closed)

The 2026-07-04 review found that only the authorization predicate `AuthorizeAdminCancel`
(`internal/control/lifecycle.go`) exists and is unit-tested. There is no HTTP route, no registry mapping
`request_id` to a running request, and no path that signals cancellation to a running dispatch. `docs/planning/26`
lists `POST /requests/{request_id}/cancel` as a P0 runtime admin endpoint and task 10 lists "admin cancel" as a
required cancellation source, so the feature is specified but unreachable. Client-disconnect and deadline
cancellation are already implemented in `dispatcher.go` `readResponse`; this task adds the admin-initiated path only.

## Required Planning Docs

- `docs/planning/09-canonical-request-lifecycle.md` (cancellation rules, terminal handling)
- `docs/planning/12-nats-protocol.md` (`CancelFrame` on the `c2e` subject)
- `docs/planning/26-config-management-api-surface.md` (runtime admin endpoint + tenant-scoped cancel rule)
- `docs/planning/14-canonical-error-registry.md` (`cancelled`, `insufficient_permissions`)
- `docs/planning/30-testing-matrix.md` (Cancellation and Worker admin rows)

## Prerequisites

- Task 10 completed (`AuthorizeAdminCancel`, lifecycle constants).
- Task 24 completed (dispatch pipeline the registry hooks into).

## Out of Scope

- Do not implement client-facing cancel APIs beyond the admin endpoint.
- Do not add streaming REST, proxy, CONNECT, or P1/P2 cancellation semantics.
- Do not add durable/persisted cancellation state; the registry is in-process (single-Control P0, consistent with the
  task 13 worker-runtime-state deferral).

## Expected Files

- Create: `internal/control/inflight.go` (in-flight request registry: `request_id -> {tenant_id, cancel func}`).
- Modify: `internal/control/dispatcher.go` (register on dispatch start, deregister on completion; cancel triggers the
  existing `ctx`-driven `CancelFrame` path).
- Modify: `internal/control/worker_handlers.go` (add `CancelRequest` handler) or a new `request_admin_handlers.go`.
- Modify: `cmd/control/main.go` (register the route and wire the registry into the dispatcher and handler).
- Test: `internal/control/inflight_test.go`, cancel-handler test, and an end-to-end cancel test extending the
  existing dispatcher round-trip harness.

## Steps

- [x] Read all required planning docs.
- [x] Add an in-flight registry: on dispatch start, store `request_id -> (tenant_id, cancelFunc)`; deregister when
      the request completes or errors (defer-based).
- [x] Add `CancelRequest` handler: authenticate, look up the request, apply `AuthorizeAdminCancel` against the stored
      tenant. A tenant-scoped caller cancelling a foreign or unknown request receives `insufficient_permissions`
      without confirming existence; `system_admin` may cancel any request.
- [x] Trigger cancellation so the running dispatch cancels its context, publishes a `CancelFrame` on the
      request-scoped `c2e` subject, and returns the canonical `cancelled` outcome to the original REST caller.
- [x] Register `POST /api/v1/admin/requests/{request_id}/cancel` in `serveAdminRoutes`.
- [x] Update `docs/agents/testing-matrix-audit.md` so the "admin cancel" Cancellation/Worker-admin rows map to the
      real endpoint test, not only `AuthorizeAdminCancel`.
- [x] Add tests for: platform `system_admin` cancels any request; tenant `tenant_admin` and `operator` keys cancel an
      own-tenant request (the full `docs/planning/26` role column: `system_admin`, `tenant_admin`, `operator`); tenant
      admin cancelling a foreign request gets `insufficient_permissions` with no existence disclosure; unknown
      `request_id` behaves identically for tenant scope; a cancelled in-flight request terminates the REST call with
      `cancelled` and a `CancelFrame` reaches the executor.
- [x] Run the focused tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control`
- `make check`

## Acceptance Criteria

- The route exists and is wired to the live dispatcher registry.
- Cancelling an in-flight request causes the original REST request to end with the canonical `cancelled` error and a
  `CancelFrame` to be published on the request's `c2e` subject.
- Foreign-tenant and unknown-request cancels from a tenant-scoped key return `insufficient_permissions` without
  disclosing whether the request exists; `system_admin` can cancel any request, and tenant-scoped `tenant_admin` and
  `operator` keys can cancel own-tenant requests.
- The testing-matrix admin-cancel rows are backed by an endpoint test, not only the authorization predicate.

## Handoff Notes

- Document the registry lifetime, the cancel-to-`CancelFrame` path, and how test time/NATS is controlled.
- Note that the registry is intentionally in-process for single-Control P0.

## Stop Conditions

- Stop before adding durable cancellation state or multi-Control coordination.
- Stop if a cancellation outcome would have no canonical error-code mapping.
- Stop if a deferral would have no owning task file.

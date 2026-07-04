# 36 - Canonical Config Base Path Normalization

Status: done

## Objective

Serve every durable config endpoint under the canonical `/api/v1/config` base path from `docs/planning/26`, removing
the bare root-path registrations.

## Context (gap being closed)

The 2026-07-04 review follow-up found `serveAdminRoutes` (`cmd/control/main.go`) registers the identity and
limits endpoints at bare root paths — `POST /tenants`, `POST|GET /platform-api-keys`,
`POST /platform-api-keys/{id}/revoke`, `POST|GET /api-keys`, `POST /api-keys/{id}/revoke`,
`POST|GET /worker-credentials`, `POST /worker-credentials/{id}/revoke`, `GET /quotas`, `PUT /tenants/{id}/quotas`,
`GET|PUT /rate-limits` — while routing-rules, deny-rules, injection-policies, and fingerprint-profiles are correctly
registered under `/api/v1/config`. `docs/planning/26` sets `/api/v1/config` as the canonical durable config base path
and `docs/planning/a-reconciliation-notes.md` records `/api/v1` as the single public API base path. No external
clients exist yet, so the bare paths move outright; no aliases or redirects. Runtime admin endpoints already live under
`/api/v1/admin` and are not touched.

Tasks 29, 30, and 31 add further config routes and register them only under the canonical path; land this task first
or expect a small rebase (shared `cmd/control/main.go`).

## Required Planning Docs

- `docs/planning/26-config-management-api-surface.md` (canonical base paths, endpoint table)
- `docs/planning/07-public-api-surface.md` (public surface shape)
- `docs/planning/a-reconciliation-notes.md` (single `/api/v1` base path)

## Prerequisites

- Tasks 18, 19, and 20 completed (the endpoints being moved exist).

## Out of Scope

- Do not add, remove, or change endpoint behavior, RBAC, or payloads; this is a path move only.
- Do not add aliases, redirects, or deprecation handling for the old bare paths.
- Do not touch the `/api/v1/admin` runtime endpoints or `POST /api/v1/requests`.

## Expected Files

- Modify: `cmd/control/main.go` (`serveAdminRoutes` patterns move under `/api/v1/config`).
- Modify: `internal/control/admin_handlers_test.go` and any other test whose request paths exercise the mux patterns.
- Modify: `docs/agents/testing-matrix-audit.md` and `deploy/docker/README.md` only if they name the old paths.

## Steps

- [x] Read all required planning docs.
- [x] Move the bare registrations under `/api/v1/config` (for example `POST /api/v1/config/tenants`,
      `PUT /api/v1/config/tenants/{id}/quotas`, `GET /api/v1/config/rate-limits`), keeping method and handler
      unchanged.
- [x] Update tests that route through the mux; grep the repo for remaining bare-path literals and fix any hits.
- [x] Verify the old bare paths now return 404 and the canonical paths behave identically to before.
- [x] Run the focused tests.
- [x] Run `make check`.
- [x] Write a handoff note.

## Tests

- `go test ./internal/control ./cmd/...`
- `make check`

## Acceptance Criteria

- Every implemented P0 config endpoint from the `docs/planning/26` table is served only under `/api/v1/config`.
- Runtime admin endpoints remain under `/api/v1/admin`; `POST /api/v1/requests` is unchanged.
- No test, doc, or deploy file references the removed bare paths.

## Handoff Notes

- List the moved routes and confirm no behavior or RBAC change rode along.

## Stop Conditions

- Stop if a path move would change handler behavior (for example a `PathValue` name mismatch) rather than just the
  registration pattern; fix the pattern, not the handler.
- Stop if a deferral would have no owning task file.

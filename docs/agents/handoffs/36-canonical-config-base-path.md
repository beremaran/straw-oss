# Handoff

Task: `docs/tasks/p0/36-canonical-config-base-path.md`

## Changed

- `cmd/control/main.go` (`serveIdentityRoutes`, `serveConfigResourceRoutes`): moved every bare-root config
  registration under `/api/v1/config`, method and handler unchanged:
  - `POST /tenants` -> `POST /api/v1/config/tenants`
  - `POST|GET /platform-api-keys` -> `POST|GET /api/v1/config/platform-api-keys`
  - `POST /platform-api-keys/{id}/revoke` -> `POST /api/v1/config/platform-api-keys/{id}/revoke`
  - `POST|GET /api-keys` -> `POST|GET /api/v1/config/api-keys`
  - `POST /api-keys/{id}/revoke` -> `POST /api/v1/config/api-keys/{id}/revoke`
  - `POST|GET /worker-credentials` -> `POST|GET /api/v1/config/worker-credentials`
  - `POST /worker-credentials/{id}/revoke` -> `POST /api/v1/config/worker-credentials/{id}/revoke`
  - `GET /quotas` -> `GET /api/v1/config/quotas`
  - `PUT /tenants/{id}/quotas` -> `PUT /api/v1/config/tenants/{id}/quotas`
  - `GET|PUT /rate-limits` -> `GET|PUT /api/v1/config/rate-limits`
  - Routing-rules, executor-pools, deny-rules, injection-policies, fingerprint-profiles, and `changes` were already
    canonical and untouched. `/api/v1/admin/*` and `POST /api/v1/requests` untouched.
- `cmd/control/admin_routes_test.go` (new): registers `serveAdminRoutes` on a bare mux and asserts every canonical
  path routes to a handler (`mux.Handler` returns a non-empty pattern) and every old bare path now 404s
  (`mux.ServeHTTP` returns `http.StatusNotFound`). This is the only place that exercises `serveAdminRoutes` through
  the mux; the handler tests below all call handler methods directly.
- `internal/control/admin_handlers_test.go`: updated request-path string literals passed to `newAdminRequest` to the
  canonical paths for accuracy. These tests call `AdminHandlers` methods directly (not through the mux), so the
  literal never drove routing — `{id}` values are set explicitly via `req.SetPathValue`, not path parsing. No
  behavior change; done for consistency and to close the "grep for remaining bare-path literals" step.
- `deploy/docker/README.md`: updated the one prose reference to `/worker-credentials` to
  `/api/v1/config/worker-credentials`.

No handler behavior, RBAC, or payload shape changed. No new dependency. No fakes, stubs, or TODOs introduced.

## Verification

```sh
go test ./internal/control ./cmd/...
make check
```

Result: both pass. `golangci-lint run --max-issues-per-linter 0 --max-same-issues 0` reports 0 issues (one `noctx`
finding in the new test was fixed by using `httptest.NewRequestWithContext`).

## Reviewer Start Points

- `cmd/control/main.go:589` (`serveIdentityRoutes`) and `cmd/control/main.go:608` (`serveConfigResourceRoutes`)
- `cmd/control/admin_routes_test.go` (new canonical/bare-path coverage)

## Remaining Work

- None. This task is a path-move only; no endpoint behavior, RBAC, or payload changed, and nothing was deferred.

## Blockers

- None.

# Straw Management UI

Browser-based control surface for the Straw Proxy management API.

## Quick Start

```sh
cd web/management
yarn install
yarn dev        # Start Vite dev server on localhost:5173
yarn build      # Production build to dist/
yarn test       # Run Vitest tests
yarn preview    # Preview production build locally
```

## Development

| Command | Description |
| --- | --- |
| `yarn dev` | Start Vite dev server (localhost:5173) with HMR |
| `yarn build` | Build production bundle to `dist/` |
| `yarn test` | Run all tests (jsdom environment) |
| `yarn preview` | Preview production build on localhost:4173 |

The dev server proxies to the Straw relay node (default `http://localhost:8081`). Configure the target URL in the sign-in screen.

## Authentication & Token Storage

- The management token is a single Bearer token configured on the relay node.
- The UI stores the token in `localStorage` only when the user checks "Remember this connection".
- **Warning**: Credentials stored in `localStorage` may be accessible to browser extensions. For shared machines, uncheck "Remember" before signing in.
- The token is sent as an `Authorization: Bearer <token>` header on all `/management/*` requests.
- Tokens are never sent to `/healthz` or other non-management endpoints.
- Signing out clears the in-memory state and removes stored credentials.

## Pages

| Route | Description |
| --- | --- |
| `#/overview` | Dashboard with endpoint health, routing attention, usage summary, cache status |
| `#/api-keys` | API key list, creation, raw-key capture, revoke |
| `#/routing-rules` | Routing rule list with filters and detail |
| `#/routing-rules/new` | Create a new routing rule |
| `#/routing-rules/edit` | Edit or duplicate an existing routing rule |
| `#/endpoints` | Active endpoint nodes, status, drain |
| `#/fingerprints` | Fingerprint preset list, JSON editor, broadcast |
| `#/usage` | Usage summary, billing estimate, date filters, CSV export |
| `#/cache` | Cache stats, Redis INFO viewer, pattern-based cache clear |
| `#/system` | Health check, capability detection, documentation links, backend gaps |

## Backend Gaps (First Release)

These features are documented in the spec but not yet exposed as UI controls:

- User accounts, SSO, and per-role permissions (no auth endpoints exist)
- Audit-log viewer (backend writes `admin_audit_log` but no read endpoint exists)
- API key update, rotation, reactivation, or explicit expiration editing
- Endpoint creation, deletion, undrain, restart, or live log viewing
- Fingerprint deletion
- Cost multiplier management (API exists but not exposed in UI)
- Saved reports, scheduled exports, alerts, and notification preferences
- Hourly drilldown, invoices, payments, and organizations in usage

These gaps are also listed on the System Info page and in the UI spec.

## Troubleshooting

### 401 Unauthorized

- Verify the management token is correct and not expired.
- Check that the relay node has `MANAGEMENT_TOKEN` configured.
- The UI automatically redirects to login on 401 responses from `/management/*` routes.

### Network / CORS Failures

- Ensure the relay node allows CORS from the dev server origin (`http://localhost:5173`).
- For production, configure the relay's CORS settings to allow the hosting origin.
- Check browser dev tools Network tab for the specific CORS error.

### Unavailable Cache Controls

- The cache stats and clear endpoints require Redis to be configured on the relay node.
- When Redis is unavailable, the Cache Control page shows an "Unavailable" notice.
- Check that the relay node has Redis connection settings configured.

### Missing Usage Summaries

- Usage data requires the usage summary job to be running on the relay node.
- If no data appears, verify the scheduled job is active and the date range covers traffic.
- Billing estimates may be empty if the usage summary job has not yet processed data for the selected range.

## Deployment

The production build outputs static files to `dist/`. These can be served by any static file server:

```sh
npx serve dist/
```

Or configure your web server (nginx, Caddy, etc.) to serve `dist/` and proxy `/management/*` requests to the relay node.

## Architecture

- **Framework**: Vanilla JavaScript with Vite for bundling and dev server.
- **Routing**: Hash-based routing (`#/overview`, `#/api-keys`, etc.).
- **State**: Single global state object with subscription-based re-rendering.
- **API Client**: Thin wrapper around `fetch` with auth header injection and error normalization.
- **Testing**: Vitest with jsdom environment.

## Links

- [Management UI Specification](../../docs/management-ui-spec.md)
- [Management API Documentation](../../docs/management-api.md)
- [OpenAPI Reference](../../api/openapi.yaml)
- [Architecture Documentation](../../docs/architecture.md)
- [Task Tracker](../../docs/management-ui-tasks/tracker.md)

# Management UI Specification

Status: ready for implementation

Last verified against the repository on 2026-06-29.

This document specifies the Straw Proxy Management UI: information architecture, screens, workflows, form behavior, validation, action states, and API mapping for a browser-based console over the existing Management API.

## Source Coverage

The spec was researched and cross-checked against:

- `README.md` for product positioning, default ports, and operator workflows.
- `docs/architecture.md` for control-plane, execution-plane, Redis, NATS, usage, and audit concepts.
- `docs/management-api.md` and `api/openapi.yaml` for public Management API behavior.
- `internal/server/admin/server.go` for the authoritative route registration.
- `internal/server/admin/handlers/*.go` for request handling, pagination, side effects, status codes, and destructive actions.
- `internal/server/dto/*.go` and `internal/domain/*.go` for data shapes and field semantics.
- `internal/infra/postgres/migrations/*.sql` for persisted entities, audit tables, usage summaries, and soft-delete behavior.
- `internal/infra/redis/endpoint_health.go` and `internal/service/endpoint/health.go` for endpoint health states, heartbeat TTL, and drain behavior.

## Product Goal

The Management UI gives operators a single control surface to administer client API keys, routing rules, endpoint health, fingerprint presets, usage, billing estimates, and Redis cache behavior. The UI should be quiet, dense, and operational. It is not a marketing site and should open directly into the console after authentication.

The first release should avoid promising unsupported backend capabilities. Where the UX naturally needs a feature but the current API does not expose it, this spec marks it as a backend dependency instead of placing it in the main workflow.

## Users

| User | Primary jobs | UI needs |
| --- | --- | --- |
| Platform operator | Keep the relay and workers healthy, drain endpoints, clear cache, inspect live state | Fast health signals, safe destructive actions, clear stale-data states |
| Scraping engineer | Create API keys, tune routing rules, configure fingerprints and filters | Accurate forms, rule previews, tag discovery, copyable examples |
| Finance or operations user | Inspect usage and estimated billing by date range and API key | Date filters, totals, data tables, exportable views |
| Read-only observer | Monitor endpoint status and usage | Dashboard and lists without mutation controls when roles exist later |

Current backend auth is a single management Bearer token. The UI can model roles in its component architecture, but every authenticated user has the same backend authority until role-aware APIs exist.

## Scope

### In Scope

- Management API connection and token sign-in.
- Overview dashboard assembled from existing endpoints.
- API key list, create, raw-key capture, and revoke.
- Routing-rule list, details, create, edit, duplicate, activate/deactivate through full update, priority editing, and soft delete.
- Endpoint list, status inspection, filtering, tag grouping, and drain action.
- Fingerprint preset list, create/update, JSON editor, and broadcast.
- Usage summary and billing estimate with date and API-key filters.
- Cache stats and pattern-based cache clearing.
- Global loading, empty, error, unauthenticated, confirmation, stale-data, and partial-failure states.
- Accessibility, responsive behavior, and implementation notes.

### Out Of Scope Until Backend Support Exists

- User accounts, SSO, per-role permissions, and session refresh.
- Audit-log viewer. The backend writes `admin_audit_log`, but no read endpoint exists.
- API key update, rotation without creating a new key, reactivation, explicit expiration editing, or scoped key detail endpoint.
- Endpoint creation, deletion, undrain, restart, or live log viewing.
- Fingerprint deletion.
- Cost multiplier management.
- Saved reports, scheduled exports, alerts, and notification preferences.

## API Coverage Matrix

| Surface | API | UI actions |
| --- | --- | --- |
| Sign-in | `GET /healthz`, authenticated calls to `/management/*` | Save base URL and token, verify connection, handle `401` |
| API keys | `GET /management/api-keys`, `POST /management/api-keys`, `DELETE /management/api-keys/{id}` | List, paginate, create, copy raw key once, revoke |
| Routing rules | `GET /management/rules`, `GET /management/rules/{id}`, `POST /management/rules`, `PUT /management/rules/{id}`, `DELETE /management/rules/{id}` | List, detail, create, edit, duplicate, toggle active, change priority, soft delete |
| Endpoints | `GET /management/endpoints`, `POST /management/endpoints/{id}/drain` | List, filter, inspect, drain |
| Fingerprints | `GET /management/fingerprints`, `POST /management/fingerprints`, `POST /management/fingerprints/broadcast` | List, create, update by same ID, broadcast |
| Usage | `GET /management/usage/summary`, `GET /management/billing/estimate` | Filter, chart, table, estimate |
| Cache | `GET /management/cache/stats`, `POST /management/cache/clear?pattern=...` | Inspect raw Redis info, clear matching keys |
| Health | `GET /healthz` | Connection check and system status |

## Information Architecture

Primary routes:

| Route | Page | Purpose |
| --- | --- | --- |
| `/login` | Connection sign-in | Enter Management API base URL and Bearer token |
| `/overview` | Overview | Operational snapshot and shortcuts |
| `/api-keys` | API keys | Provision and revoke client credentials |
| `/routing-rules` | Routing rules | Manage traffic selection, filters, fingerprints, and endpoint pools |
| `/routing-rules/new` | New routing rule | Guided create form |
| `/routing-rules/:id` | Rule detail/edit | Inspect and modify one rule |
| `/endpoints` | Endpoints | Monitor live workers and drain nodes |
| `/fingerprints` | Fingerprints | Manage browser/TLS spoofing presets |
| `/usage` | Usage and billing | Daily request, byte, cost-unit, and estimate views |
| `/cache` | Cache | Redis stats and cache clearing |
| `/system` | System | Connection details, health, backend gaps, and diagnostics |

The default post-login route is `/overview`. Direct deep links should redirect to `/login` only when no valid connection details are present.

## App Shell

Use a work-focused console layout:

- Left navigation with labels: Overview, API Keys, Routing Rules, Endpoints, Fingerprints, Usage, Cache, System.
- Top bar with environment label, Management API base URL, connection status, refresh button, and sign-out menu.
- Main content uses page headers with one primary action at the right.
- Tables are dense but readable: sticky header, compact rows, visible sort state, and pagination controls.
- Mutating actions use confirmation dialogs when they revoke, delete, drain, broadcast, or clear cache.
- Toasts report short outcomes. Long details stay in inline alert panels or expandable error detail.

Responsive behavior:

- Desktop: persistent side nav, multi-column dashboard, detail drawers or split panes where helpful.
- Tablet: collapsible side nav, forms keep section order.
- Mobile: bottom or drawer navigation, tables become cards with the same actions, destructive actions remain behind confirmation.

## Authentication And Connection

### Sign-In Screen

Fields:

- Management API URL, default `http://localhost:8081`.
- Management token, entered as password text.
- Remember this connection toggle. When enabled, persist base URL and token in browser storage with a warning that local storage is readable by browser extensions. When disabled, keep them in memory for the tab session.

Actions:

- Connect: call `GET /healthz` first for reachability, then an authenticated lightweight request such as `GET /management/api-keys?limit=1`.
- Sign out: clear stored token and in-memory query cache.

Validation and errors:

- URL is required and must include protocol.
- Token is required. Although the configuration reference calls `MANAGEMENT_API_KEY` optional, `internal/server/admin/middleware/auth.go` requires a Bearer token for every `/management/*` request.
- `401` shows "Invalid management token" and keeps the user on sign-in.
- Network/CORS failures show the attempted URL and suggest checking `MANAGEMENT_PORT`, TLS, and CORS.

Security requirements:

- Never log or display the management token after entry.
- Add `Authorization: Bearer <token>` only to `/management/*` requests.
- Raw API keys returned from creation are handled separately and never persisted automatically.

## Global Data Behavior

- Default page size is 20. The backend caps list `limit` at 100.
- Every page has a manual refresh button and a visible "Last refreshed" timestamp.
- Mutations invalidate related queries immediately.
- Destructive mutation dialogs name the affected object and explain the backend effect.
- Backend error payloads use `{ "error": "...", "code": "...", "details": ... }`; show `error` first and expose details in a disclosure.
- For operations that can be repeated client-side across multiple rows, such as bulk revoke, show a progress counter and partial-failure report.

## Overview

The Overview page is assembled from existing APIs. It is not a separate backend endpoint.

Cards:

- Endpoint health: counts by `healthy`, `suspect`, `unhealthy`, `draining`, plus total active tasks.
- Routing rules: total rules, active rules, highest priority active rule, and count of inactive soft-deleted rules.
- API keys: total keys, active keys, revoked/inactive keys.
- Usage: total requests and bytes for the last 7 days, using `GET /management/usage/summary`.
- Billing: month-to-date cost units and estimated USD, using `GET /management/billing/estimate`.
- Cache: Redis availability and short status from `GET /management/cache/stats`.

Main sections:

- "Endpoint pressure" table: endpoint ID, state, active tasks, tags, last seen, drain action.
- "Recent usage" chart: daily requests and bytes for the last 7 days.
- "Routing attention" list: inactive rules, duplicate priorities, rules with no required tags, rules referencing missing fingerprint presets, and rules whose endpoint pools reference endpoints not in the live list.

Empty state:

- If all aggregate requests fail because auth fails, return to sign-in.
- If a subset fails, show cards for successful data and an inline "Some panels could not load" warning with retry.

## API Keys

### List

API: `GET /management/api-keys?page=&limit=`

Columns:

- Name.
- Status: Active or Revoked.
- Scopes, displayed as chips with overflow count.
- Rate limit override.
- Created.
- Expires, if present.
- ID, copyable.
- Row actions.

Filters:

- Status: all, active, revoked. Implement client-side for loaded pages.
- Scope contains. Implement client-side for loaded pages.
- Search by name or ID. Implement client-side for loaded pages.

Actions:

- Create API key.
- Revoke key.
- Copy key ID.
- Copy scopes.
- Bulk revoke selected active keys by issuing sequential `DELETE` calls, with confirmation and partial-failure reporting.

### Create API Key

API: `POST /management/api-keys`

Fields:

| Field | Control | Validation |
| --- | --- | --- |
| Name | Text input | Required, trimmed, 1 to 120 characters |
| Scopes | Token/chip editor | Each scope is `*`, exact tag such as `type:residential`, prefix wildcard such as `target:*`, or suffix wildcard such as `*:us` |
| Rate limit override | Number input | Empty or integer greater than 0 |

UX details:

- Offer scope suggestions from live endpoint tags and routing-rule tags.
- Show examples near the scope editor without blocking advanced patterns.
- On success, show a raw-key modal before returning to the list.

Raw-key modal:

- Title: "Save this API key now".
- Display `raw_key` in a masked field with reveal, copy, and download text-file actions.
- Explain that the raw key is returned once and cannot be recovered.
- Disable Close until the user checks "I have saved this key".
- After close, never display `raw_key` again.

### Revoke API Key

API: `DELETE /management/api-keys/{id}`

Behavior:

- The backend sets `is_active=false`; records remain in lists.
- Confirmation copy states that clients using the key lose access immediately.
- After success, update the row to Revoked without removing it.
- If the backend returns not-found through a server error, refresh the list and show "The key may already have been removed or revoked".

Backend gap:

- No update, reactivation, expiration-edit, or rotate endpoint exists. Rotation should be modeled as create a new key, distribute it, then revoke the old key.

## Routing Rules

Routing rules are the most important management surface. The UI should make advanced fields accessible without overwhelming routine edits.

### List

API: `GET /management/rules?page=&limit=`

Columns:

- Priority.
- Name.
- Status.
- Required tags.
- Excluded tags.
- Fingerprint mode: none, preset, or A/B.
- Rate limits.
- Timeout.
- Quota key.
- Version.
- Updated.
- Row actions.

Sorting:

- Default order follows backend order: priority descending, then creation time descending.
- Client-side sorting can be offered for loaded rows, but changing actual matching order requires editing priority.

Filters:

- Status: all, active, inactive.
- Required tag contains.
- Fingerprint preset.
- Quota key.
- Priority range.
- Search by name or ID.

Actions:

- Create.
- Open details.
- Edit.
- Duplicate. This copies the rule into the create flow with ID, timestamps, and version removed.
- Toggle active. Implement as full `PUT /management/rules/{id}` with current version and changed `is_active`.
- Change priority. Implement as full update; warn that higher numbers evaluate first.
- Delete. The backend soft-deletes by setting `is_active=false`.

Attention indicators:

- Duplicate active priority.
- Active rule with no required tags.
- Invalid tag syntax.
- Referenced fingerprint preset not present in `GET /management/fingerprints`.
- Endpoint pool references no currently listed endpoint.
- `allow_insecure_tls=true`.
- Request filters disabled and no blocked domains or patterns.

### Rule Detail

API: `GET /management/rules/{id}`

Tabs:

- Summary: ID, status, priority, version, created, updated, quota key, core tags.
- Match and routing: required/excluded tags, endpoint types, capabilities, pools.
- Limits and TLS: timeout, per-minute/per-second rate limits, insecure TLS, pinned certificate hash.
- Fingerprints: preset or A/B config.
- Request filters: adblock, lists, blocked content types, URL patterns, domains.
- Raw JSON: read-only API payload and a copy button.

### Create And Edit Form

Use a sectioned form with a right-side summary panel on desktop and a sticky footer on smaller screens.

Required basic fields:

| Field | API field | UX |
| --- | --- | --- |
| Name | `name` | Required text |
| Active | `is_active` | Switch, default on for create |
| Priority | `priority` | Integer stepper, default 0 |
| Quota key | `quota_key` | Optional text, suggestions from usage quota keys when available |

Match fields:

| Field | API field | Validation |
| --- | --- | --- |
| Required tags | `required_tags` | Tag chips. Use `key:value` or `key=value`; normalize display to `key:value`. Bare `*` is not valid for routing rules because it will not match normal request tags. |
| Excluded tags | `excluded_tags` | Same tag validation. Excluded tags must not duplicate required tags. |

Limits:

| Field | API field | Validation |
| --- | --- | --- |
| Hard timeout | `hard_timeout` | Empty or Go duration string such as `30s`, `1m`, `1m30s` |
| Rate limit per minute | `rate_limit_per_minute` | Empty or integer greater than 0 |
| Rate limit per second | `rate_limit_per_second` | Empty or integer greater than 0 |

Endpoint constraints:

| Field | API field | UX |
| --- | --- | --- |
| Allowed endpoint types | `allowed_endpoint_types` | Chip editor, suggestions from tags with key `type` |
| Required endpoint capabilities | `required_endpoint_caps` | Chip editor, suggestions from endpoint tags and capability naming used by operators |
| Endpoint pools | `endpoint_pools` | Repeatable groups with tier integer, endpoint ID multi-select, and max retries |

Fingerprinting:

| Mode | API fields | UX |
| --- | --- | --- |
| None | No fingerprint fields | Leave unset |
| Preset | `fingerprint_preset` | Select from fingerprint presets, show config preview |
| A/B test | `fingerprint_ab_test` | Variant table with preset ID and weight, strategy text/select |

A/B validation:

- At least two variants.
- Each variant references an existing preset unless the user confirms an unknown preset.
- Each weight is a positive integer.
- Show total weight and resulting percentages.

Request filters:

| Field | API field | UX |
| --- | --- | --- |
| Enable AdBlock | `request_filters.enable_adblock` | Switch |
| AdBlock lists | `request_filters.adblock_lists` | Multi-line URL/text list |
| Block content types | `request_filters.block_content_types` | Chip editor with MIME suggestions |
| Block URL patterns | `request_filters.block_url_patterns` | Multi-line pattern list |
| Block domains | `request_filters.block_domains` | Domain chip editor |

TLS/security:

| Field | API field | UX |
| --- | --- | --- |
| Allow insecure TLS | `allow_insecure_tls` | Switch with high-friction warning |
| Pinned certificate hash | `pinned_cert_hash` | Text input, validate as hash-like string and warn on unusual length |

Advanced editor:

- Provide a "Raw JSON" mode for advanced users.
- The raw editor must stay synchronized with the form when valid.
- Invalid JSON blocks save and highlights the parse error.
- On create, submit only the request DTO fields, not ID or timestamps.
- On update, include `version` from the loaded rule.

Save behavior:

- Create: `POST /management/rules`.
- Edit: `PUT /management/rules/{id}` with the current `version`.
- If update returns a version/not-found style error, refetch the rule and offer "Review latest version" and "Overwrite after review". Do not silently retry.
- After any create, update, or delete, the backend increments the rule-cache version when Redis rule cache exists. The UI should still invalidate its own cached list immediately.

Delete behavior:

- Label as "Deactivate rule" in most places, with secondary copy explaining the backend stores it as inactive.
- Use "Delete rule" only in the confirmation title if matching API language matters.
- After success, keep the row visible when filter is all/inactive and mark it Inactive.

## Endpoints

### List

API: `GET /management/endpoints`

Columns:

- Endpoint ID.
- State: `healthy`, `suspect`, `unhealthy`, `draining`.
- Active tasks.
- Tags.
- Version.
- Last seen.
- Actions.

Filters:

- State.
- Tag.
- Version.
- Search by endpoint ID.
- Last seen age: active, stale, older than 60 seconds.

Status rules:

- Health records have a default Redis TTL of 60 seconds.
- `healthy` and `suspect` are selectable by the routing service.
- `draining` means the endpoint should finish active work and refuse new work.
- Missing endpoints should be shown through empty/stale states, not invented from Postgres, because the current list endpoint reads live health records.

Actions:

- Drain endpoint: `POST /management/endpoints/{id}/drain`.
- Copy endpoint ID.
- Copy tag set.
- Open filtered routing-rule list for rules that may match the endpoint.

Drain confirmation:

- State that active tasks are allowed to finish and new work should stop.
- Show current active task count.
- Disable if endpoint already shows `draining`.
- After success, optimistically set state to Draining and refresh.

Backend gaps:

- There is no undrain endpoint. To clear draining state, an operator currently needs backend/Redis intervention or a future API.
- There is no endpoint create, delete, restart, logs, or metrics detail endpoint.

## Fingerprint Presets

### List

API: `GET /management/fingerprints`

Columns:

- ID.
- Name.
- Browser family, inferred from ID/config when possible.
- User agent, if present in config.
- Updated.
- Used by rules count, calculated from routing rules.
- Actions.

Actions:

- Create preset.
- Edit preset. The existing `POST /management/fingerprints` upserts by ID.
- Duplicate preset into a new ID.
- Copy config JSON.
- Broadcast all presets.

### Create Or Edit Preset

API: `POST /management/fingerprints`

Fields:

| Field | API field | Validation |
| --- | --- | --- |
| ID | `id` | Required, stable slug; lock during edit because posting same ID updates |
| Name | `name` | Required |
| Config | `config` | JSON object |

Config editor:

- Use a JSON code editor with formatting, linting, and copy.
- Provide optional helper fields for common keys such as `user_agent`, `ja3`, `h2_settings`, header order, and pseudo-header order, but keep the stored shape flexible because backend accepts arbitrary JSON object.
- Show built-in preset IDs from code knowledge as suggestions only when present in repository docs or bundled seed data. Do not assume the database has them unless `GET /management/fingerprints` returns them.

Broadcast:

- API: `POST /management/fingerprints/broadcast`.
- Confirm before broadcasting because it pushes all registered presets to active workers through NATS.
- Show success as "Broadcast requested"; the current API does not return worker acknowledgment counts.
- If broker is unavailable, show the backend error and keep existing presets untouched.

Backend gap:

- The repository has delete methods, but no Management API route exposes preset deletion. The UI must not show a destructive delete action for fingerprints in the first release.

## Usage And Billing

### Usage Summary

API: `GET /management/usage/summary?start=&end=&api_key_id=`

Controls:

- Date range: last 7 days, last 30 days, month to date, custom.
- API key filter: select from `GET /management/api-keys`.
- Granularity: daily only, because the backend summary endpoint is daily.

Metrics:

- Total requests.
- Total bytes, formatted as B, KB, MB, GB.
- Cost units.
- Breakdown by endpoint tier from `breakdown`.

Visuals:

- Line or bar chart for daily requests.
- Stacked bar for endpoint-tier breakdown.
- Table with date, requests, bytes, cost units, and breakdown.

Export:

- Provide client-side CSV export of the loaded data.
- Include date range and API key ID in the filename.

### Billing Estimate

API: `GET /management/billing/estimate?start=&end=&api_key_id=`

Display:

- Total cost units.
- Estimated USD.
- Currency.
- Date range.

Important copy:

- The current backend calculation is `estimated_usd = total_cost_units * 0.0001`.
- Label this as an estimate, not an invoice.

Validation:

- Dates must be `YYYY-MM-DD`.
- Start must be on or before end.
- If a date parse error returns `400`, keep the current filters and highlight the invalid field.

Empty state:

- "No usage has been summarized for this range." Show likely causes: new installation, no traffic, or summary job not populated.

Backend gaps:

- No hourly drill-down endpoint.
- No cost multiplier management endpoint.
- No invoice, payment, or organization model.

## Cache

### Stats

API: `GET /management/cache/stats`

Display:

- Redis availability.
- Raw `INFO` text in a searchable, copyable panel.
- Derived quick facts if parsed client-side: Redis version, used memory, connected clients, keyspace hits/misses.

Unavailable state:

- Cache routes are only registered when the server has a Redis client. If `GET /management/cache/stats` returns not found or fails consistently while other management routes work, show "Cache controls are unavailable in this server configuration."

### Clear Cache

API: `POST /management/cache/clear?pattern=...`

Fields:

- Pattern, default `*`.

Confirmation:

- Pattern `*` requires typing `CLEAR ALL`.
- Any other pattern requires confirming the pattern text.
- Explain that cache clearing forces the relay to rebuild cached rules, keys, health, or other Redis-backed state from source systems as applicable.

Success:

- Show `deleted` count and pattern from the response.
- Refresh stats.

Error:

- If scan or delete fails, show the backend error and do not claim partial count unless returned.

## System

The System page gathers connection and diagnostics that do not belong to a domain page.

Sections:

- Management API base URL and sign-out.
- Health: `GET /healthz` result and response time.
- Backend capabilities detected: cache controls available, fingerprints available, usage available.
- Documentation links: Management API, OpenAPI reference, architecture.
- Known backend gaps from this spec.

Do not show secrets on this page.

## Global UX States

| State | Required behavior |
| --- | --- |
| Loading | Skeleton rows/cards matching final layout; keep previous data visible during refresh when possible |
| Empty | Explain the entity, provide the primary creation action when supported |
| Unauthorized | Clear token from request cache, show sign-in with "Session token rejected" |
| Network error | Show URL, method, retry action, and connection settings shortcut |
| Server error | Show backend `error`, expandable `code` and `details`, retry |
| Validation error | Keep form data, focus first invalid field, show field-level messages |
| Mutation success | Toast and update local data immediately |
| Partial failure | Show completed and failed items, with retry failed only |
| Stale data | Show last refresh time and a visible refresh action |
| Destructive pending | Disable repeated clicks and show operation progress |

## Validation Rules

Tags:

- Routing tags use `key:value` or `key=value`.
- Normalize display to `key:value`.
- Key is required.
- Empty values are allowed by backend parsing, but the UI should warn because they are rarely useful.
- Routing-rule tags should not use bare `*`; API-key scopes may use `*`.

Scopes:

- Allow exact tags such as `region:us`.
- Allow `*` for all scopes.
- Allow prefix wildcard such as `target:*`.
- Allow suffix wildcard such as `*:us`.
- Warn when a scope cannot match any known endpoint tag or routing tag, but allow it for future tags.

Durations:

- Use Go duration strings accepted by `time.ParseDuration`: `500ms`, `30s`, `1m`, `1h`.
- Reject natural-language values such as `30 seconds`.

Numbers:

- Page size: 1 to 100.
- Priority: integer.
- Rate limits: integer greater than 0 when set.
- Endpoint pool tier: integer.
- Max retries: integer greater than or equal to 0.
- A/B weight: integer greater than 0.

Dates:

- Use `YYYY-MM-DD`.
- Treat backend dates as server-local unless a timezone is returned.

JSON:

- Fingerprint config must be an object.
- Routing raw editor must produce a payload compatible with create/update DTO fields.

## Accessibility

- All controls require visible labels.
- Icon-only actions require tooltips and `aria-label`.
- Destructive dialogs must move focus into the dialog and restore focus to the trigger after close.
- Tables must expose row headers and selected state to assistive technology.
- Charts require a data table alternative on the same page.
- Color cannot be the only state signal; pair status colors with text labels.
- Keyboard support is required for navigation, menus, dialogs, chip editors, and JSON editor escape.

## Visual Design Guidance

The UI should feel like an operational console:

- Use restrained neutral surfaces with clear status colors.
- Avoid marketing-style hero sections, decorative cards, or oversized headings.
- Use compact controls and predictable placement.
- Prefer tables for operational lists and cards only for dashboard metrics or repeated mobile row summaries.
- Use icons for common actions: refresh, copy, create, edit, delete/deactivate, filter, download, broadcast, drain.
- Keep destructive actions visually distinct and never next to the primary save action without spacing or grouping.

## Implementation Notes

- Use generated OpenAPI types if the frontend stack supports it. The contract is `api/openapi.yaml`.
- Keep a small API client layer responsible for base URL, auth header injection, JSON parsing, and error normalization.
- Use optimistic UI only after the backend request succeeds for destructive actions, except endpoint drain may optimistically mark Draining while refresh is pending because it is a direct state request.
- Store query keys by endpoint plus parameters so date filters and pagination do not collide.
- When creating routing rules from duplicate, omit `id`, `version`, `created_at`, and `updated_at`.
- When updating routing rules, always include the currently loaded `version`; the backend increments it.
- Deleting a routing rule is a soft delete through `is_active=false`, so the UI should support inactive filters.
- `GET /management/cache/stats` returns raw Redis info as a string, not structured hit-rate fields.
- The management token should be considered required despite configuration documentation wording.

## First-Release Acceptance Checklist

- Sign-in verifies both reachability and management-token authorization.
- Every registered Management API route has a visible UI surface or a documented reason for being system-only.
- API key creation forces raw-key capture before dismissing success.
- API key revoke, endpoint drain, fingerprint broadcast, routing-rule delete, and cache clear all require confirmation.
- Routing-rule create/edit includes every field in `CreateRoutingRuleRequest` and `UpdateRoutingRuleRequest`.
- Routing-rule update handles version conflicts by refetching and requiring user review.
- Endpoint list shows all backend states and makes draining irreversible in the current UI copy.
- Fingerprint editor supports arbitrary JSON object configs and upsert semantics.
- Usage and billing filters send `YYYY-MM-DD` dates and optional `api_key_id`.
- Cache clear supports a pattern and high-friction confirmation for `*`.
- Empty, loading, unauthorized, network error, server error, validation error, and partial-failure states are implemented consistently.
- The UI does not expose unsupported first-release actions as if the backend supports them.


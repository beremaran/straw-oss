# MU-002: Management API Client And Connection Model

Status: done
Phase: 1
Depends on: MU-001
Search tags: api client, base URL, bearer token, error normalization, OpenAPI, pagination, query keys

## Objective

Add the shared client layer that all UI pages use to call the Management API safely and consistently.

## Scope

- Store connection settings as base URL plus in-memory or remembered token.
- Call `GET /healthz` without a management auth header.
- Add `Authorization: Bearer <token>` only to `/management/*` requests.
- Normalize backend error payloads shaped as `{ "error": "...", "code": "...", "details": ... }`.
- Support page and limit parameters, with default limit 20 and maximum 100.
- Use generated OpenAPI types from `api/openapi.yaml` if the frontend stack supports it without excessive setup.
- Define query keys by endpoint plus parameters so filters and pagination do not collide.

## Repo Touchpoints

- `api/openapi.yaml`
- `web/management/src/api/*`
- `web/management/src/state/*`
- `web/management/src/types/*`

## Implementation Tasks

- [x] Create a fetch wrapper for JSON requests, auth injection, and response parsing.
- [x] Add typed helpers for health, API keys, routing rules, endpoints, fingerprints, usage, billing, and cache routes.
- [x] Preserve method, URL, status, backend `error`, `code`, and `details` in normalized errors.
- [x] Add pagination helpers for list endpoints.
- [x] Add a small mocked-client check that fails if auth is sent to `/healthz` or omitted from `/management/*`.

## Done Criteria

- [x] Shared API helpers cover every endpoint listed in the UI spec API coverage matrix.
- [x] `401` responses can be recognized by callers for sign-in redirect behavior.
- [x] Network and CORS failures preserve the attempted URL and method for display.
- [x] No page component builds its own ad hoc Management API request.


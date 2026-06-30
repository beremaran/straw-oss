# MU-007: API Key List, Creation, Raw-Key Capture, And Revoke

Status: done
Phase: 2
Depends on: MU-005
Search tags: `/api-keys`, `raw_key`, revoke, scopes, rate limit override, bulk revoke, copy ID

## Objective

Implement the API key management surface for listing, creating, capturing raw keys, revoking, and bulk revoking.

## Scope

- List API keys from `GET /management/api-keys?page=&limit=`.
- Show name, status, scopes, rate limit override, created date, expiration, copyable ID, and row actions.
- Add client-side filters for status, scope contains, and search by name or ID.
- Create keys with name, scopes, and rate limit override through `POST /management/api-keys`.
- Offer scope suggestions from live endpoint tags and routing-rule tags.
- Show raw-key modal with masked reveal, copy, download, and required "I have saved this key" checkbox.
- Revoke keys through `DELETE /management/api-keys/{id}` and keep revoked records visible.
- Bulk revoke selected active keys sequentially with confirmation and partial-failure reporting.

## Repo Touchpoints

- `web/management/src/routes/api-keys*`
- `web/management/src/components/api-keys/*`
- `web/management/src/api/apiKeys*`

## Implementation Tasks

- [ ] Build paginated dense table and mobile card equivalent.
- [ ] Add create form validation for name, scopes, and rate limit override.
- [ ] Prevent raw key modal dismissal until the save acknowledgement is checked.
- [ ] Never store `raw_key` outside transient UI state.
- [ ] Handle not-found/server-error revoke responses by refreshing and showing the spec copy.

## Done Criteria

- [ ] API key create, raw-key capture, revoke, and bulk revoke work through existing APIs.
- [ ] Raw API keys are shown only immediately after creation.
- [ ] Revoked keys remain in the list and show Revoked status.
- [ ] Unsupported update, rotate, reactivation, and expiration-edit actions are not exposed.


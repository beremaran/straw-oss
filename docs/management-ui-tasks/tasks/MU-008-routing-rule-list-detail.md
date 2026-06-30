# MU-008: Routing Rule List, Detail View, Filters, And Attention Indicators

Status: done
Phase: 2
Depends on: MU-005
Search tags: `/routing-rules`, rule detail, priority, required tags, endpoint pools, fingerprints, raw JSON, attention

## Objective

Implement the routing rule list and detail view, including filters and client-side attention indicators.

## Scope

- List routing rules from `GET /management/rules?page=&limit=`.
- Preserve backend default order: priority descending, then creation time descending.
- Show priority, name, status, required/excluded tags, fingerprint mode, rate limits, timeout, quota key, version, updated date, and row actions.
- Add filters for status, required tag contains, fingerprint preset, quota key, priority range, and search by name or ID.
- Add details route from `GET /management/rules/{id}`.
- Add detail tabs for summary, match and routing, limits and TLS, fingerprints, request filters, and raw JSON.
- Add copy button for raw API payload.
- Detect duplicate active priority, active rule without required tags, invalid tag syntax, missing fingerprint preset, missing endpoint pool match, insecure TLS, and disabled request filters.

## Repo Touchpoints

- `web/management/src/routes/routing-rules*`
- `web/management/src/components/routing-rules/*`
- `web/management/src/api/routingRules*`

## Implementation Tasks

- [ ] Build routing rules table and mobile card equivalent.
- [ ] Implement client-side filters for loaded rows.
- [ ] Load fingerprints and endpoints needed for attention indicators.
- [ ] Build detail tabs with the fields named in the spec.
- [ ] Add duplicate action entry point that sends sanitized rule data into the create flow.

## Done Criteria

- [ ] Routing rule list and detail cover every display field in the spec.
- [ ] Attention indicators are visible without blocking valid backend data.
- [ ] Raw JSON is read-only in detail view and copyable.
- [ ] Duplicate does not carry `id`, `version`, `created_at`, or `updated_at`.


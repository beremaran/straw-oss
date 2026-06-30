# MU-009: Routing Rule Create, Edit, Duplicate, And Version-Conflict Handling

Status: done
Phase: 2
Depends on: MU-008
Search tags: routing rule form, `CreateRoutingRuleRequest`, `UpdateRoutingRuleRequest`, version, A/B, raw JSON, deactivate

## Objective

Implement the routing rule create/edit form with every first-release request field and safe update behavior.

## Scope

- Create rules through `POST /management/rules`.
- Update rules through `PUT /management/rules/{id}` with the currently loaded `version`.
- Support duplicate flow with sanitized source data.
- Include basic fields, match fields, limits, endpoint constraints, fingerprint mode, A/B variants, request filters, TLS/security, and raw JSON mode.
- Keep raw JSON mode synchronized with the form while JSON is valid.
- Block save and highlight parse errors when raw JSON is invalid.
- Toggle active and change priority as full updates.
- Deactivate/delete rules through the existing delete API and copy that explains soft-delete/inactive behavior.
- Handle version/not-found style update errors by refetching and requiring user review before overwrite.

## Repo Touchpoints

- `web/management/src/routes/routing-rules/new*`
- `web/management/src/routes/routing-rules/:id*`
- `web/management/src/components/routing-rules/editor*`
- `web/management/src/validation/routingRules*`

## Implementation Tasks

- [ ] Build sectioned form with desktop summary panel and smaller-screen sticky footer.
- [ ] Validate required tags, excluded tags, durations, rate limits, endpoint pools, A/B weights, and pinned certificate hash.
- [ ] Submit only create/update DTO fields; omit server-owned fields on create.
- [ ] Include loaded `version` on update.
- [ ] Add review-latest-version flow for conflict-like errors.

## Done Criteria

- [ ] The editor covers every field in `CreateRoutingRuleRequest` and `UpdateRoutingRuleRequest`.
- [ ] Save, duplicate, active toggle, priority change, and deactivate use existing Management API routes.
- [ ] Version conflicts are never silently retried.
- [ ] Inactive rules remain visible when filters include inactive/all.


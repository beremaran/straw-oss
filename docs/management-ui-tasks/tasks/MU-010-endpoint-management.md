# MU-010: Endpoint List, Stale-State Handling, And Drain Workflow

Status: done
Phase: 2
Depends on: MU-005
Search tags: `/endpoints`, drain, healthy, suspect, unhealthy, draining, health TTL, stale, tags, active tasks

## Objective

Implement endpoint monitoring and the one supported endpoint mutation: drain.

## Scope

- List endpoints from `GET /management/endpoints`.
- Show endpoint ID, state, active tasks, tags, version, last seen, and actions.
- Filter by state, tag, version, search by endpoint ID, and last seen age.
- Treat health records as live Redis-backed data with a default 60-second TTL.
- Show missing or stale endpoints through empty/stale states, not invented Postgres records.
- Drain endpoint through `POST /management/endpoints/{id}/drain`.
- Copy endpoint ID and tag set.
- Link to filtered routing-rule list for rules that may match the endpoint.

## Repo Touchpoints

- `web/management/src/routes/endpoints*`
- `web/management/src/components/endpoints/*`
- `web/management/src/api/endpoints*`

## Implementation Tasks

- [x] Build endpoint table and mobile card equivalent.
- [x] Add state and stale-age filters.
- [x] Add drain confirmation with active task count and backend effect copy.
- [x] Disable drain for endpoints already in `draining`.
- [x] Optimistically mark state as Draining after successful request while refresh is pending.

## Done Criteria

- [x] All backend endpoint states are visible with text labels.
- [x] Drain is confirmed, pending-state protected, and refreshes the list after success.
- [x] The UI clearly says undrain, create, delete, restart, logs, and metrics detail are not first-release actions.
- [x] No unsupported endpoint mutation is exposed.


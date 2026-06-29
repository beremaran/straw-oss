# MU-013: Cache Stats, Redis Info Viewer, And Pattern Clear Workflow

Status: not-started
Phase: 3
Depends on: MU-005
Search tags: `/cache`, Redis, cache stats, INFO, clear pattern, `CLEAR ALL`, deleted count

## Objective

Implement Redis cache inspection and pattern-based cache clearing.

## Scope

- Load stats from `GET /management/cache/stats`.
- Show Redis availability and raw `INFO` text in a searchable, copyable panel.
- Derive quick facts client-side when present: Redis version, used memory, connected clients, keyspace hits, and keyspace misses.
- Handle unavailable cache routes when Redis is not configured.
- Clear cache through `POST /management/cache/clear?pattern=...`.
- Default pattern to `*`.
- Require typing `CLEAR ALL` for `*`; require confirming pattern text for any other pattern.
- Show returned `deleted` count and pattern, then refresh stats.

## Repo Touchpoints

- `web/management/src/routes/cache*`
- `web/management/src/components/cache/*`
- `web/management/src/api/cache*`

## Implementation Tasks

- [ ] Build stats panel with raw Redis info search and copy.
- [ ] Parse quick facts defensively from raw info text.
- [ ] Add unavailable-state detection when cache routes fail but other management routes work.
- [ ] Add clear-cache form and high-friction confirmation.
- [ ] Show backend scan/delete errors without inventing partial counts.

## Done Criteria

- [ ] Cache stats and clear pattern workflows match the spec.
- [ ] `*` clear cannot run without the required confirmation phrase.
- [ ] Successful clear shows backend `deleted` count and refreshes stats.
- [ ] Cache controls are clearly unavailable when the server has no Redis client route.


# MB-012: Endpoint Management APIs

Status: not-started
Phase: 3
Depends on: MB-011
Search tags: endpoints, create, patch, delete, drain, undrain, restart, commands

## Objective

Expose endpoint registry, desired-state, command, and log status APIs.

## Scope

- Add endpoint create, detail, patch, delete, undrain, restart, command list, and command detail routes.
- Update existing drain route to record a command and desired state while preserving `200` compatibility.
- Merge persisted registry state with Redis health in detail/list responses.
- Keep deleted endpoints non-routeable even if they still heartbeat.

## Repo Touchpoints

- `internal/server/admin/handlers/endpoints.go`
- `internal/server/admin/server.go`
- `internal/server/dto/*endpoint*.go`
- `internal/service/endpoint/health.go`
- `internal/infra/postgres/*endpoint*_repo.go`
- `api/openapi.yaml`
- `docs/management-api.md`

## Implementation Tasks

- [ ] Add DTOs for endpoint registry, health, desired state, and command summaries.
- [ ] Add create, detail, patch, and delete handlers.
- [ ] Update drain handler response to include command ID when available.
- [ ] Add undrain and restart handlers.
- [ ] Add command list and detail handlers.

## Done Criteria

- [ ] Endpoint records can be created, updated, deleted, drained, undrained, restarted, inspected, and queried for commands.
- [ ] Deleting an endpoint removes Redis health and drain state.
- [ ] Deleted heartbeating endpoints are visible in detail as deleted but still heartbeating.
- [ ] Existing list and drain tests remain compatible.

# MB-011: Endpoint Registry Persistence And Command Schema

Status: done
Phase: 3
Depends on: MB-001
Search tags: endpoints, registry, desired_state, endpoint_commands, endpoint_log_entries

## Objective

Persist endpoint desired state, registration state, command status, and log rows.

## Scope

- Add endpoint table columns from the spec.
- Add `endpoint_commands`.
- Add `endpoint_log_entries`.
- Add repositories for endpoint registry, commands, and logs.
- Preserve Redis health as live state while Postgres stores management intent.

## Repo Touchpoints

- `internal/infra/postgres/migrations/*.sql`
- `internal/domain/endpoint.go`
- `internal/infra/postgres/*endpoint*_repo.go`
- `internal/infra/redis/endpoint_health.go`
- `internal/service/endpoint/health.go`

## Implementation Tasks

- [x] Add migration for endpoint columns, commands, and log entries.
- [x] Add endpoint registry domain fields for desired state, registered state, deleted state, and metadata.
- [x] Add command domain model with statuses from the spec.
- [x] Add log-entry model and cursor-friendly query support.
- [x] Add repository tests for desired state, soft delete, command lifecycle, and log queries.

## Done Criteria

- [x] Endpoint registry state can be managed without Redis health data.
- [x] Command records can be created and moved through accepted, acknowledged, running, succeeded, failed, and timed_out.
- [x] Log entries can be queried by endpoint and time in descending order.

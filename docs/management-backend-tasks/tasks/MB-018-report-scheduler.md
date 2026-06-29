# MB-018: Report Scheduler

Status: not-started
Phase: 5
Depends on: MB-017
Search tags: report_schedules, cron, timezone, row locking, scheduler, worker

## Objective

Run due report schedules safely from the relay or a worker process.

## Scope

- Add schedule list, create, patch, and delete/disable endpoints.
- Claim due schedules with row locking.
- Create report runs for claimed schedules.
- Update `next_run_at`, `last_run_at`, run status, artifact URL, and error fields.
- Keep storage local by default with an interface for S3-compatible storage later.

## Repo Touchpoints

- `internal/server/admin/handlers/*report*.go`
- `internal/service/*report*.go`
- `internal/infra/postgres/*report*_repo.go`
- `cmd/relay/main.go`
- `internal/config/config.go`

## Implementation Tasks

- [ ] Add schedule DTOs, handlers, and route registration.
- [ ] Add cron and timezone validation.
- [ ] Add due-schedule query using row locking.
- [ ] Add scheduler loop with configurable interval.
- [ ] Add tests for due claiming, disabled schedules, and run status updates.

## Done Criteria

- [ ] Schedules can be created, updated, listed, and disabled/deleted.
- [ ] Concurrent scheduler workers do not claim the same due schedule.
- [ ] Successful and failed scheduled runs record status and timestamps.

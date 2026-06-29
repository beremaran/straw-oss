# MB-014: Endpoint Logs

Status: not-started
Phase: 3
Depends on: MB-011, MB-013
Search tags: endpoint logs, `endpoints:logs`, SSE, retention, cursor, log forwarding

## Objective

Forward, store, query, and stream endpoint logs for management users.

## Scope

- Add worker log forwarding to `endpoint.logs.<endpoint_id>` when enabled.
- Store forwarded logs in `endpoint_log_entries`.
- Add `GET /management/endpoints/{id}/logs`.
- Add `GET /management/endpoints/{id}/logs/stream` using SSE.
- Enforce retention: 7 days or 5 GB, whichever comes first.

## Repo Touchpoints

- `pkg/endpoint/worker.go`
- `internal/server/admin/handlers/endpoints.go`
- `internal/server/admin/server.go`
- `internal/infra/postgres/*endpoint*_repo.go`
- `internal/observability/logging/logger.go`
- `internal/config/config.go`

## Implementation Tasks

- [ ] Add config flag `ENDPOINT_LOG_STREAM_ENABLED`.
- [ ] Add log payload type and worker publisher.
- [ ] Add relay-side subscriber to persist logs.
- [ ] Add cursor-paginated log query handler with filters from the spec.
- [ ] Add SSE stream handler for live tail.
- [ ] Add retention cleanup job.

## Done Criteria

- [ ] Log query supports `start`, `end`, `level`, `q`, `trace_id`, `request_id`, `cursor`, and `limit`.
- [ ] Limit is capped at 500.
- [ ] SSE live stream works without WebSocket support.
- [ ] Retention cleanup removes old or excessive log data.

# MB-014: Endpoint Logs

Status: done
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

- [x] Add config flag `ENDPOINT_LOG_STREAM_ENABLED`.
- [x] Add log payload type and worker publisher.
- [x] Add relay-side subscriber to persist logs.
- [x] Add cursor-paginated log query handler with filters from the spec.
- [x] Add SSE stream handler for live tail.
- [x] Add retention cleanup job.

## Done Criteria

- [x] Log query supports `start`, `end`, `level`, `q`, `trace_id`, `request_id`, `cursor`, and `limit`.
- [x] Limit is capped at 500.
- [x] SSE live stream works without WebSocket support.
- [x] Retention cleanup removes old or excessive log data.

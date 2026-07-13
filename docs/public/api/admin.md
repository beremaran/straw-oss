# Admin API reference

The Admin API exists only when runtime administration is enabled. Every request requires
`Authorization: Bearer <admin-token>`; use `X-Straw-Actor` on mutations for audit attribution. Keep this surface on a
separate protected network. Errors use `{"error":"stable_or_diagnostic_value"}`.

| Method and path | Purpose | Concurrency and result |
| --- | --- | --- |
| `GET /api/v1/admin/config` | Current snapshot | Returns snapshot, revision, and `ETag` |
| `PUT /api/v1/admin/config` | Validate and activate a complete snapshot | Requires quoted/numeric `If-Match`; returns `ConfigRecord` and new `ETag` |
| `GET /api/v1/admin/config/history` | Bounded newest-first audit history | Read-only |
| `POST /api/v1/admin/config/rollback` | Reapply a historical `config_version` as a new version | Requires `If-Match` and `{"config_version":N}`; creates a new audited version |
| `GET /api/v1/admin/rollouts` | Worker acknowledgement state | Read-only snapshot |
| `GET /api/v1/admin/workers` | Workers ordered by worker ID | Read-only |
| `POST /api/v1/admin/workers/{worker_id}/{action}` | `drain`, `undrain`, `disable`, or `enable` | Requires `If-Match`; validates worker/action and returns new record/ETag |
| `GET /api/v1/admin/requests` | Active owned requests | Read-only; HA data depends on shared runtime state |
| `DELETE /api/v1/admin/requests/{request_id}` | Request cancellation | Safe to repeat; ownership is resolved through shared state in HA |

`ConfigRecord` contains `snapshot`, `actor`, `action`, `created_at`, and `revision`. History is `{"items":[ConfigRecord...]}`
in newest-first bounded order. Rollout is `config_version`, Control status, and ordered worker entries containing
`worker_id`/`status` (`pending` or `applied`). Workers return ordered items with identity/session/state, enabled,
draining, executor type, active/available/max capacity, last seen, and pools. Active requests contain only
`request_id` and deployment ID.

Mutation bodies are limited to 4 MiB and must be one strict JSON value without unknown fields or trailing data.
Missing `If-Match` returns 428 `if_match_required`; malformed values return 400 `invalid_if_match`; stale revisions
return 409 `revision_conflict`; invalid snapshots return 422; missing cancellation targets return 404
`request_not_found`; bad/missing auth returns 401 `admin_auth_required`. Successful cancellation returns 204 without
a body. Store/backend failures are non-2xx and do not activate partial configuration.
The dashboard at `/admin/` is an API client, not an additional security boundary.

See [Configuration](../configuration.md) for the complete snapshot schema and
[Runtime administration](../runtime-administration.md) for tested workflows.

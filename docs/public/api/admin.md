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

## Examples

Read the current record and preserve its quoted revision before making a mutation:

```sh
export ADMIN_URL=http://127.0.0.1:8080
export ADMIN_TOKEN=replace-me
curl -fsS -D /tmp/straw-admin-headers -o /tmp/straw-config.json \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$ADMIN_URL/api/v1/admin/config"
etag=$(awk 'tolower($1)=="etag:" {gsub("\r", "", $2); print $2}' /tmp/straw-admin-headers)
cat /tmp/straw-config.json
```

The saved response has the shape `{"snapshot":{...},"revision":N,"actor":"...","action":"...","created_at":"..."}`.
Send the complete `snapshot` object—not the outer record—when updating it:

```sh
curl -fsS -X PUT \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "X-Straw-Actor: operator@example.com" \
  -H "If-Match: $etag" \
  -H 'Content-Type: application/json' \
  --data-binary @snapshot.json \
  "$ADMIN_URL/api/v1/admin/config"
```

Inspect audit history, rollout acknowledgements, worker state, and active requests with the same admin bearer token:

```sh
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$ADMIN_URL/api/v1/admin/config/history"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$ADMIN_URL/api/v1/admin/rollouts"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$ADMIN_URL/api/v1/admin/workers"
curl -fsS -H "Authorization: Bearer $ADMIN_TOKEN" "$ADMIN_URL/api/v1/admin/requests"
```

Rollback and worker lifecycle actions are also compare-and-swap mutations. Refresh `ETag` after every successful
mutation and use a new request if another operator changed the revision:

```sh
curl -fsS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "X-Straw-Actor: operator@example.com" \
  -H "If-Match: $etag" \
  -H 'Content-Type: application/json' \
  -d '{"config_version":3}' \
  "$ADMIN_URL/api/v1/admin/config/rollback"

curl -fsS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "X-Straw-Actor: operator@example.com" \
  -H "If-Match: $etag" \
  "$ADMIN_URL/api/v1/admin/workers/egress-1/drain"
```

## Status and error mapping

| HTTP status | Error or result | Meaning and next action |
| --- | --- | --- |
| `200` | record/list/rollout | The read or mutation succeeded; mutation responses include a new quoted `ETag`. |
| `204` | cancellation | The active request was cancelled; there is no response body. |
| `401` | `admin_auth_required` | The admin token is missing or invalid. |
| `400` | `invalid_if_match`, JSON decode error, unknown action, or store diagnostic | Fix the request or inspect the server diagnostic; no snapshot is activated. |
| `404` | `request_not_found` | The cancellation target is not active in the owned runtime state. |
| `409` | `revision_conflict` | Re-read the current config, merge deliberately, and retry with its `ETag`. |
| `422` | invalid runtime snapshot | Fix the complete snapshot against [Configuration](../configuration.md); durable and published state is unchanged. |
| `428` | `if_match_required` | Add the current `ETag` value as `If-Match`. |
| `500` | backend/configuration diagnostic | Repair the enabled JetStream/runtime-state dependency and retry only after checking whether the request committed. |

`If-Match` accepts either the quoted `ETag` value or its numeric contents. A successful worker action can create a
durable setting for a worker ID that is not currently registered; it still does not make that worker eligible until it
registers and passes admission checks. `GET /api/v1/admin/requests` lists only request ID and deployment ID, so use
the request client or logs for detailed request context.

See [Configuration](../configuration.md) for the complete snapshot schema and
[Runtime administration](../runtime-administration.md) for tested workflows.

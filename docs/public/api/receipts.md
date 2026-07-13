# Receipt API reference

Receipt endpoints exist only with object storage enabled and use the deployment request bearer token. IDs are
deployment-scoped. All client lifecycle routes below require `Authorization: Bearer <request-token>`; the worker-only
assignment route uses its signed query parameters instead and does not accept a bearer token. Signed assignment URLs
are short-lived bearer credentials and must not be logged.

| Method and path | Contract |
| --- | --- |
| `POST /api/v1/receipts` | Create with `direction` (`request` or `response`), non-negative `size_bytes` (zero is valid), 64-character hexadecimal `sha256_hex`, and optional `idempotency_key`; returns 201 |
| `GET /api/v1/receipts/{id}` | Return state, declared size/checksum, parts, and expiry |
| `DELETE /api/v1/receipts/{id}` | Mark a receipt cancelled and remove its payload; assigned/consumed receipts return 409, and repeating cancellation is safe; returns 200 with the record |
| `PUT /api/v1/receipts/{id}/parts/{part}` | Upload or replace a positive part number in any order with an exact `Content-Length` and optional `X-Straw-Part-SHA256` |
| `POST /api/v1/receipts/{id}/complete` | Compose parts and verify exact byte count and SHA-256 before readiness |
| `GET /api/v1/receipts/{id}/content` | Download an authorized ready response receipt before expiry |
| `GET /api/v1/receipt-objects/{id}` | Assignment-scoped worker read; requires valid signature, assignment, and expiry |

JSON responses contain `receipt_id`, `direction`, `state`, `size_bytes`, `sha256_hex`, `parts`, `created_at`,
`updated_at`, `expires_at`, and optional assignment/failure fields. They also include `status_url`; uploading records
include `part_upload_template` and `complete_url`, while verified/consumed response records include `download_url`.
Parts contain `number`, `size_bytes`, and `sha256_hex`.

The lifecycle is `uploading → verifying → verified → assigned → consumed`; `rejected`, `cancelled`, and `expired` are
terminal for assignment and completion. A failed assignment may return `assigned → verified`. The cancellation
endpoint is idempotent for an already-cancelled record and currently marks any record other than `assigned` or
`consumed` as `cancelled`. Illegal transitions return 409, unknown IDs 404, malformed metadata/parts 400, deployment
authentication 401, invalid assignment signatures 403, and unexpected storage failures 500. Error JSON is
`{"code":"receipt_error","message":"..."}`. Oversized metadata is rejected at 64 KiB; parts and final objects use
configured limits. Checksum or size mismatch produces `rejected`, never verified. Create and completion are safe to
retry only with the same declared identity and content. A receipt is never proof of content until completion
verification succeeds.

Part numbers may be uploaded out of order or replaced after an interruption. Completion is where Control requires the
uploaded set to be exactly `1..N` and verifies the aggregate byte count and SHA-256. A part may have size zero, but its
`Content-Length` must still be sent explicitly.

Content downloads return `application/octet-stream`, exact `Content-Length`, and `X-Straw-SHA256` for authorized
response receipts. Assignment-object downloads require `deployment_id`, `request_id`, `expires`, and `signature`
query parameters and return exact content length; those URLs are worker capabilities, not client download URLs.

HA deployments require every Control instance to use the same object store and record/index persistence. See
[Object storage and receipts](../object-storage-receipts.md) for multipart, corruption, cancellation, and cleanup
examples.

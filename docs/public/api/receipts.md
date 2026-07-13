# Receipt API reference

Receipt endpoints exist only with object storage enabled and use the deployment request bearer token. IDs are
deployment-scoped. Signed assignment URLs are short-lived bearer credentials and must not be logged.

| Method and path | Contract |
| --- | --- |
| `POST /api/v1/receipts` | Create with `direction` (`request` or `response`), positive `size_bytes`, 64-character hexadecimal `sha256_hex`, and optional `idempotency_key`; returns 201 |
| `GET /api/v1/receipts/{id}` | Return state, declared size/checksum, parts, and expiry |
| `DELETE /api/v1/receipts/{id}` | Cancel a non-consumed receipt; repeated cancellation is safe |
| `PUT /api/v1/receipts/{id}/parts/{part}` | Upload a positive, contiguous part number with an exact `Content-Length` and optional `X-Straw-Part-SHA256` |
| `POST /api/v1/receipts/{id}/complete` | Compose parts and verify exact byte count and SHA-256 before readiness |
| `GET /api/v1/receipts/{id}/content` | Download an authorized ready response receipt before expiry |
| `GET /api/v1/receipt-objects/{id}` | Assignment-scoped worker read; requires valid signature, assignment, and expiry |

JSON responses contain `receipt_id`, `direction`, `state`, `size_bytes`, `sha256_hex`, `parts`, `created_at`,
`updated_at`, `expires_at`, and optional assignment/failure fields. They also include `status_url`; uploading records
include `part_upload_template` and `complete_url`, while verified/consumed response records include `download_url`.
Parts contain `number`, `size_bytes`, and `sha256_hex`.

The lifecycle is `uploading → verifying → verified → assigned → consumed`; `rejected`, `cancelled`, and `expired` are
terminal. A failed assignment may return `assigned → verified`. Illegal transitions return 409, unknown IDs 404,
malformed metadata/parts 400, deployment authentication 401, invalid assignment signatures 403, and unexpected
storage failures 500. Error JSON is `{"code":"receipt_error","message":"..."}`. Oversized metadata is rejected at
64 KiB; parts and final objects use configured limits. Checksum or size mismatch produces `rejected`, never verified.
Create and completion are safe to retry only with the same declared identity and content. A receipt is never proof of
content until completion verification succeeds.

Content downloads return `application/octet-stream`, exact `Content-Length`, and `X-Straw-SHA256` for authorized
response receipts. Assignment-object downloads require `deployment_id`, `request_id`, `expires`, and `signature`
query parameters and return exact content length; those URLs are worker capabilities, not client download URLs.

HA deployments require every Control instance to use the same object store and record/index persistence. See
[Object storage and receipts](../object-storage-receipts.md) for multipart, corruption, cancellation, and cleanup
examples.

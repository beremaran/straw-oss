---
sidebar_position: 11
---

# Troubleshooting

## `make dev` does not become ready

Run:

```sh
make dev-status
make dev-logs
```

Check for occupied ports, Docker build failures, and NATS health. Use the port overrides in the
[deployment guide](deployment.md#local-development) when another service owns 4222, 8222, 8080, or 9090.

## `auth_failure`

The deployment has `STRAW_AUTH_TOKEN` set and the request omitted or supplied the wrong bearer token. Send exactly:

```text
Authorization: Bearer <STRAW_AUTH_TOKEN>
```

## `route_unavailable` or assignment errors

No worker is currently ready or it has no capacity. Check Egress logs/readiness and NATS connectivity. Start or scale
workers, then retry only replayable requests.

## `invalid_request`

Confirm that the body is JSON, `method` and an absolute HTTP/HTTPS `url` are present, headers and bodies are base64,
the timeout is within configured limits, and no unknown fields are present.

## Response body appears unreadable

`body.data_base64` is base64, not text. Decode it with `base64 --decode` or your language's base64 library. A
`body_too_large` error means the configured response limit was exceeded; increase it deliberately or fetch a smaller
resource.

## `body_ref_unavailable` or a receipt conflict

Check `GET /api/v1/receipts/{id}`. Only a `verified` request receipt can be assigned. `uploading` needs remaining
parts and completion; `rejected` means the final size or checksum did not match; `assigned` is already in use; and
`cancelled` or `expired` cannot be revived. Confirm `download_base_url` is reachable from Egress and that every
Control shares the same signing key and durable object store.

For S3 errors, verify endpoint/bucket/region, path-style compatibility, credentials, clock synchronization, and the
configured server-side-encryption permissions. Interrupted uploads remain resumable until retention cleanup.

## NATS payload errors

Keep NATS `max_payload`, Control/Egress `max_payload_bytes`, and Control `max_frame_data_bytes` compatible. The
production example provides a known-good 2 MB setup.

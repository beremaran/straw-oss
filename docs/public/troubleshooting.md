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

## Diagnostic runbook matrix

Use this sequence for every incident: identify the symptom, run the non-destructive fast check, confirm one cause,
apply the narrow fix, then collect only sanitized escalation data.

| Symptom | Fast check | Likely causes and confirmation | Fix |
| --- | --- | --- | --- |
| health is up but readiness is down | `curl -fsS localhost:9090/readyz`; inspect component JSON logs | NATS/Redis unavailable, startup snapshot missing, draining shutdown; confirm backend health and `straw_runtime_state_available` | restore the dependency or configuration; never bypass readiness in the load balancer |
| worker absent/saturated | inspect `straw_worker_sessions`, `straw_workers_available`, heartbeat age and `/api/v1/admin/workers` | NATS credentials, protocol/tag mismatch, capacity exhausted, disabled/draining worker | correct compatible tag/credentials or add capacity; undrain only after work is safe |
| high latency/timeouts | compare request, routing, assignment, and NATS histograms | no capacity, slow DNS/TLS/upstream, deadline too small, NATS delay | isolate the stage, scale or repair it, then adjust a documented limit deliberately |
| DNS/TLS/redirect failure | reproduce with the same destination policy from Egress network | denied resolved IP/CNAME, missing CA, SNI/certificate, redirect to denied target, proxy mismatch | fix DNS/cert/policy or proxy; do not disable post-resolution enforcement |
| HTTP/2/fingerprint failure | inspect error code and executed profile evidence | unsupported worker profile, peer HTTP/2 incompatibility, fallback cache | use a supported compatible profile or ordinary TLS; preserve fallback evidence |
| Redis HA degradation | check Redis and `straw_runtime_state_*` | TLS/auth/network failure, expired ownership, slow operation | restore Redis before admitting traffic; allow fencing/TTLs to converge |
| JetStream rollout stalls | inspect `/api/v1/admin/rollouts` and NATS JetStream health | worker offline/incompatible, bucket unavailable, invalid snapshot acknowledgement | restore bucket/worker and roll back through the Admin API if required |
| receipt/S3 failure | inspect receipt state and receipt counters | credentials/permissions, clock skew, retention, missing part, checksum/size mismatch | repair storage/time/config; retry resumable parts or create a new rejected receipt |
| disk pressure | inspect JetStream and receipt volume usage | retention/history too large, cleanup stopped, log growth | preserve required backup, restore cleanup, then resize or shorten reviewed retention |
| shutdown hangs | inspect readiness and active request count after SIGTERM | upstream ignores deadline, unavailable NATS, tunnel still active | wait for documented grace period; terminate only after accepting request loss |
| partial upgrade/protocol mismatch | compare Control/Egress/protocol/SDK versions with compatibility matrix | wrong upgrade order or unsupported mixed minor | finish Egress-first upgrade or roll back by immutable digest |
| CLI/SDK error | run equivalent documented curl request | base URL/token, serialization, timeout, incompatible SDK tag | correct environment/tag and handle typed API errors; do not blindly retry writes |
| Docker platform startup failure | `docker compose config`; inspect image architecture and volume ownership | unsupported architecture, occupied port, read-only path, Docker DNS | use supported image/platform, port overrides, and documented UID/volume permissions |

## Safe escalation bundle

Run `make diagnostic-bundle PROFILE=default` (or `admin`, `receipts`, or `ha`) and redirect its output to a file if
needed. The collector emits only an allowlist of tool/revision versions, the selected profile, and numeric local
health/readiness status. It never reads raw environment, configuration, metrics, logs, request data, or command
arguments. `make diagnostic-bundle-check` injects synthetic token, URL, header, body, and backend-credential values
and proves none can enter the output.

Provide the exact Straw/component versions and digests, deployment profile, OS/architecture, timestamps, request IDs,
health/readiness results, relevant metric values, and a redacted configuration **shape**. Replace every token, URL,
hostname, IP, header, body, signed receipt URL, and backend credential before sharing. Do not attach raw environment
variables, object data, packet captures, or production logs. If redaction cannot be verified, describe the observation
instead of uploading the artifact.

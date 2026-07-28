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

## Ingress `407 Proxy Authentication Required`

Forward-proxy ingress authenticates with `Proxy-Authorization`; do not put the proxy credential in the destination's
ordinary `Authorization` header:

```sh
curl --proxy http://localhost:8080 \
  --proxy-header "Proxy-Authorization: Bearer $STRAW_AUTH_TOKEN" \
  https://example.com
```

The proxy credential is stripped before a decoded request reaches the destination. A destination `Authorization`
header is end-to-end application data and is a separate secret.

## Upstream proxy failures

An ingress `407` happens before routing and means the client did not authenticate to Control. By contrast, a public
`upstream_proxy_failure` with `upstream_status: 407` means Egress reached the configured provider gateway and the
provider rejected its worker-local Basic authentication. Use the fixed `details.fact` and optional status without
collecting credentials:

| Safe fact | `upstream_status` | Check |
| --- | --- | --- |
| `upstream_proxy_connect_failed` | absent | Gateway DNS, route, firewall, explicit host/port, and provider reachability from Egress. |
| `upstream_proxy_tls_failed` | absent | Outer HTTPS-proxy certificate, SNI, trust roots, and clock. This is distinct from `upstream_tls_failure` after CONNECT to the target. |
| `upstream_proxy_protocol_error` | absent | Malformed, oversized, folded, or body-bearing CONNECT response; confirm gateway protocol/mode rather than relaxing parsing. |
| `upstream_proxy_authentication_failed` | `407` | Named credential variables exist, the account/zone is enabled, and the bounded `username_template` matches current provider syntax. |
| `upstream_proxy_connect_rejected` | returned non-2xx status | Provider destination ACL, account policy, quota, target port, and approved status-specific provider guidance. |

For `authentication_failed`/407 and `connect_rejected`/status incidents, record only the profile ID, fixed fact, numeric
status, phase duration, and request ID. Never print or attach the environment value, rendered username, password,
`Proxy-Authorization`, provider session ID, raw sticky ID, full destination URL/query, or proxy response headers.

Pool/profile inconsistencies fail closed rather than dialing direct. Check both sides exactly:

- Control pool `upstream_proxy.id` equals worker `capabilities.allowed_pools[].upstream_proxy_id`;
- that worker ID references one configured `upstream_proxies[].id`;
- `trusted_remote_resolution` is true, the pool uses a fresh ID, and the worker negotiated protocol minor 2;
- all old Controls have stopped and shared worker rows were expired before workers re-registered.

A stale or missing registration claim normally makes the route `route_unavailable`. Invalid/unused profiles fail worker
startup. A selected-pool/instruction mismatch returns `executor_internal_error` with safe fact
`upstream_proxy_instruction_invalid`; an executed frame identity mismatch returns `protocol_error`. Do not repair either
case by changing the proxy pool to direct under the same ID.

## `if_match_required` or `revision_conflict`

Admin mutations use compare-and-swap. Read the current config, copy its `ETag`, send that value as `If-Match`, and
refresh it after every successful mutation. A `428 if_match_required` means the header is absent; `409
revision_conflict` means another operator already changed the record.

```sh
curl -fsS -D headers.txt -o record.json \
  -H "Authorization: Bearer $STRAW_ADMIN_TOKEN" \
  http://localhost:8080/api/v1/admin/config
revision=$(awk 'tolower($1)=="etag:" {gsub("\r", "", $2); print $2}' headers.txt)
curl -fsS -X PUT \
  -H "Authorization: Bearer $STRAW_ADMIN_TOKEN" \
  -H "If-Match: $revision" \
  -H 'Content-Type: application/json' \
  --data-binary @snapshot.json \
  http://localhost:8080/api/v1/admin/config
```

## `body_too_large` or `header_injection_failed`

`body_too_large` identifies the request or response direction and may include `limit_bytes`. Reduce the inline body,
raise the reviewed static limit, or use a request/response receipt where the selected profile supports it. A response
receipt requires object storage; it does not make an inline request body unlimited.

`header_injection_failed` means a runtime injection operation produced a reserved, malformed, CR/LF-containing, or
oversized header. Check the complete snapshot's base64 values and the reserved-header list in
[Configuration](configuration.md#runtime-header-injection); do not bypass the rejection by allowing hop-by-hop or
`X-Straw-*` headers.

## TLS proxy failures

For the production HAProxy overlay, check the merged Compose configuration, certificate/key file permissions and
order, and the Control readiness path:

```sh
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.yml -f deploy/production/compose.tls.yml config
make tls-proxy-check
curl --fail-with-body --cacert /path/to/managed-ca.pem \
  "https://127.0.0.1:${STRAW_TLS_PORT:-8443}/readyz"
```

The checked-in TLS check uses an ephemeral QA certificate and is not a production certificate installation step. Keep
certificate verification enabled when reproducing a client failure.

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
| DNS/TLS/redirect failure | reproduce with the same destination mode and policy from Egress network | direct denied IP/CNAME, remote provider DNS/ACL, missing CA, SNI/certificate, redirect, proxy mismatch | fix DNS/cert/policy or provider ACL; do not pretend local DNS preflight enforces a provider-owned result |
| HTTP/2/fingerprint failure | inspect error code and executed profile evidence | unsupported worker profile, peer HTTP/2 incompatibility, fallback cache | use a supported compatible profile or ordinary TLS; preserve fallback evidence |
| Redis HA degradation | check Redis and `straw_runtime_state_*` | TLS/auth/network failure, expired ownership, slow operation | restore Redis before admitting traffic; allow fencing/TTLs to converge |
| JetStream rollout stalls | inspect `/api/v1/admin/rollouts` and NATS JetStream health | worker offline/incompatible, bucket unavailable, invalid snapshot acknowledgement | restore bucket/worker and roll back through the Admin API if required |
| receipt/S3 failure | inspect receipt state and receipt counters | credentials/permissions, clock skew, retention, missing part, checksum/size mismatch | repair storage/time/config; retry resumable parts or create a new rejected receipt |
| disk pressure | inspect JetStream and receipt volume usage | retention/history too large, cleanup stopped, log growth | preserve required backup, restore cleanup, then resize or shorten reviewed retention |
| shutdown hangs | inspect readiness and active request count after SIGTERM | upstream ignores deadline, unavailable NATS, tunnel still active | wait for documented grace period; terminate only after accepting request loss |
| partial upgrade/protocol mismatch | compare Control/Egress/protocol/SDK versions with compatibility matrix | old Control still writing shared rows, stale registrations, or a proxy claim below minor 2 | keep pools direct, finish the Control-first upgrade, expire shared worker rows, then register minor-2 workers |
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

# Configuration

Control and Egress accept `-config PATH` with a strict, versioned JSON file. Unknown fields are rejected so typos do
not silently change behavior. Input is bounded to 4 MiB before decoding. Without `-config`, both binaries use local
defaults.

## Control

```json
{
  "config_version": "v1",
  "control": {
    "server": {"host": "0.0.0.0", "api_port": 8080, "metrics_port": 9090},
    "request": {
      "max_inline_request_body_bytes": 1048576,
      "max_inline_response_body_bytes": 1048576,
      "max_timeout_ms": 120000
    },
    "transport": {"max_frame_data_bytes": 1048576},
    "nats": {"servers": ["nats://127.0.0.1:4222"]},
    "runtime_admin": {"enabled": false},
    "object_storage": {"enabled": false}
  }
}
```

Set `STRAW_AUTH_TOKEN` to require `Authorization: Bearer <token>` on REST requests and `Proxy-Authorization: Bearer
<token>` on forward-proxy requests. An unset token permits requests and is appropriate only for a loopback or
otherwise trusted development network.

Forward-proxy routing hints use the reserved `X-Straw-*` request-header namespace; they do not add configuration
fields. The supported headers, limits, normalization, and stripping behavior are documented in [HTTP and HTTPS proxy
ingress](proxy-ingress.md#routing-hints). The configured token is checked before those values are parsed. Routing rules,
executor-pool capability filters, sticky TTLs, and `allow_sticky_fallback` apply equally to REST, absolute-form proxy,
and CONNECT ingress modes.

### Optional runtime administration

`runtime_admin.enabled` opts into durable runtime configuration. Its defaults are `token_env: "STRAW_ADMIN_TOKEN"`,
`bucket: "STRAW_RUNTIME_CONFIG"`, and `history_limit: 64`. The named token environment variable must be non-empty,
and NATS must have JetStream file storage enabled. See [Runtime administration](runtime-administration.md).

### Runtime snapshot validation

The Admin API accepts a complete snapshot, not a partial patch. Before compare-and-swap persistence or activation,
Control validates route match fields, pool tags and capability lists, sticky settings, destination rules, injection
policies, fingerprint records, and worker settings. A rejected snapshot returns `422` and leaves the durable record,
active cache, history, and worker publication unchanged.

Destination policy rules use the raw representation below. The normalized fields are server-derived response fields and
must not be used as client input. `host` takes a hostname; `host_suffix` and `cname_suffix` take a hostname or a
leading `*.`/`.` suffix; `cidr` takes a CIDR; `ip` takes one IP; `private_range` takes a private CIDR; and
`metadata_ip` takes one of Straw's recognized metadata addresses. Control canonicalizes these values before returning
the stored record and clears derived fields from a previous pattern:

```json
{
  "destination_policy": [
    {"id": "deny-api", "rule_type": "host_suffix", "action": "deny", "enabled": true, "raw_pattern": "*.Example.COM"},
    {"id": "deny-net", "rule_type": "cidr", "action": "deny", "enabled": true, "raw_pattern": "192.168.1.42/24"}
  ]
}
```

The response contains `normalized_host: "example.com"` and `normalized_cidr: "192.168.1.0/24"`. Supplying malformed
patterns, unsupported actions, invalid header operations or values, duplicate policy records, or fingerprint metadata
that the official Egress worker cannot execute is rejected at PUT time. Existing snapshots that predate raw patterns
remain readable when their normalized destination field is valid.

Executor pools and routing rules are part of the runtime snapshot. A pool is deployment-scoped: its `enabled` flag,
exact `executor_type`, required `tags`, degraded-worker policy, and allowed country/region/IP-type lists are enforced
when Control selects a worker. For example, a deployment can route residential traffic to a dedicated pool:

```json
{
  "config_version": 1,
  "executor_pools": [
    {
      "id": "default",
      "executor_type": "egress",
      "enabled": true
    },
    {
      "id": "residential-au",
      "executor_type": "egress",
      "tags": ["residential"],
      "enabled": true,
      "allow_degraded_workers": false,
      "allowed_countries": ["AU"],
      "allowed_regions": ["ap-southeast-2"],
      "allowed_ip_types": ["residential"]
    }
  ],
  "routing_rules": [
    {
      "id": "residential-au",
      "priority": 10,
      "enabled": true,
      "target_pool_id": "residential-au",
      "match": {"country": "AU", "ip_type": "residential"}
    }
  ]
}
```

`enabled: false` stops new assignments to that pool while existing requests continue. A worker is eligible only when
it claims membership in the target pool, has the pool's exact executor type, advertises every required pool tag, and
keeps all claimed countries, regions, and IP types within the pool's allowed lists. A non-empty request constraint
must also be advertised by the worker. `allow_degraded_workers` controls whether a live degraded worker may be used.
Unknown pool references and duplicate pool memberships are rejected before registration or activation.

### Optional shared runtime state

`runtime_state.backend` defaults to `memory`. Set it to `redis` only when multiple Control instances must be
interchangeable. Redis credentials belong in the URL named by `redis_url_env` (default `STRAW_REDIS_URL`); both
`redis://` and TLS-protected `rediss://` URLs are accepted.

```json
{
  "runtime_state": {
    "backend": "redis",
    "redis_url_env": "STRAW_REDIS_URL",
    "key_prefix": "straw",
    "instance_id_env": "STRAW_CONTROL_INSTANCE_ID",
    "worker_ttl_ms": 30000,
    "request_ttl_ms": 130000,
    "instance_ttl_ms": 15000,
    "operation_timeout_ms": 1000
  }
}
```

`request_ttl_ms` must exceed `request.max_timeout_ms`. Instance IDs must be unique NATS subject tokens; if the named
environment variable is empty, Control generates a process-unique ID. See [Highly available Control](highly-available-control.md)
for TTL and outage behavior.

### Optional object storage

`object_storage.enabled` defaults to `false`. The `local` backend defaults to `.straw/objects`; the `s3` backend
requires `endpoint` and `bucket`. Common defaults are 1 GiB maximum objects, 16 MiB maximum parts, 24-hour retention,
five-minute assignment URLs, and hourly cleanup. `download_base_url` must be reachable by Egress.

Secrets are read from `signing_key_env` (`STRAW_RECEIPT_SIGNING_KEY`), `access_key_env`, `secret_key_env`, and
`session_token_env`; never place their values in JSON. The signing key must contain at least 32 bytes. S3 server-side
encryption accepts `AES256` or `aws:kms`; the latter also requires `kms_key_id`. See
[Object storage and receipts](object-storage-receipts.md) for the full lifecycle and examples.

## Egress

```json
{
  "config_version": "v1",
  "egress": {
    "worker_id": "egress-1",
    "heartbeat_interval_ms": 5000,
    "health_port": 8090,
    "capabilities": {
      "max_concurrency": 4,
      "allowed_pools": [
        {"pool_id": "default"},
        {"pool_id": "residential-au"}
      ]
    },
    "nats": {"servers": ["nats://127.0.0.1:4222"]},
    "upstream_connection_pool": {"enabled": false},
    "http2": {"enabled": false, "fallback_cache_ttl_ms": 300000}
  }
}
```

Worker IDs must be unique within a deployment. `max_concurrency` defaults to `4`. When `allowed_pools` is omitted,
the official worker claims `default/default`, preserving the original behavior. Each entry may omit `deployment_id`,
which defaults to `default`; entries must be unique and refer to the deployment's configured pools. The official
worker does not provide tenant or cross-deployment authorization. The optional connection pool can
reuse upstream connections; when enabled its defaults are 8 idle connections per deployment/host, 30 seconds idle
timeout, and 5 minutes maximum lifetime.

The official Egress worker advertises the complete built-in fingerprint catalogue. The default Control snapshot
enables those exact profiles with contract revision `tls-client-v1.15.1-http1-http2`; use their names in
`fingerprint_profile`. Runtime-admin snapshots may disable profiles but cannot make an unknown profile executable.
See [Compatibility](compatibility.md#fingerprint-profile-catalogue) for the normative list and PSK behavior.

## NATS authentication

Both services support:

- `user_credentials_file`: path to a NATS credentials file; or
- `username_env` and `password_env`: names of environment variables containing credentials.

The production example uses `STRAW_NATS_USER` and `STRAW_NATS_PASSWORD`. Never put secret values directly in the JSON
files. Connection tuning fields are `reconnect_attempts`, `reconnect_wait_ms`, `ping_interval_ms`,
`max_ping_failures`, and `max_payload_bytes`.

`deploy/local/*.json` and `deploy/production/*.json` are the canonical working examples.

## Static field reference

Static fields take effect only after the affected process restarts. Environment values are read at startup; JSON
stores environment-variable **names**, never secret values.

| Object | Field | Default / constraint |
| --- | --- | --- |
| file | `config_version` | required literal `v1`; exactly one Control or Egress object |
| Control server | `host`, `api_port`, `metrics_port` | `0.0.0.0`, `8080`, `9090`; ports 1–65535 |
| Control request | `max_inline_request_body_bytes`, `max_inline_response_body_bytes`, `max_timeout_ms` | 1 MiB, 1 MiB, 120000 ms; positive bounded request behavior |
| Control transport | `max_frame_data_bytes` | 1 MiB and compatible with NATS `max_payload_bytes` |
| runtime admin | `enabled`, `token_env`, `bucket`, `history_limit` | false, `STRAW_ADMIN_TOKEN`, `STRAW_RUNTIME_CONFIG`, 64; history 1–64 when enabled |
| runtime state | `backend`, `redis_url_env`, `key_prefix`, `instance_id_env` | `memory`, `STRAW_REDIS_URL`, `straw`, `STRAW_CONTROL_INSTANCE_ID`; backend `memory` or `redis` |
| runtime state TTL | `worker_ttl_ms`, `request_ttl_ms`, `instance_ttl_ms`, `operation_timeout_ms` | 30000, 130000, 15000, 1000; all positive and request TTL greater than max request timeout |
| object storage identity | `enabled`, `backend`, `local_directory`, `endpoint`, `bucket`, `region` | false, `local`, `.straw/objects`, empty, empty, `us-east-1`; S3 requires absolute endpoint and bucket |
| object storage secret names | `access_key_env`, `secret_key_env`, `session_token_env`, `signing_key_env` | `STRAW_S3_ACCESS_KEY`, `STRAW_S3_SECRET_KEY`, `STRAW_S3_SESSION_TOKEN`, `STRAW_RECEIPT_SIGNING_KEY` |
| object storage limits | `download_base_url`, `max_object_bytes`, `max_part_bytes`, `retention_seconds`, `assignment_ttl_seconds`, `cleanup_interval_seconds` | `http://control:8080`, 1 GiB, 16 MiB, 86400, 300, 3600; positive; part ≤ object |
| object encryption | `server_side_encryption`, `kms_key_id` | empty; `AES256` or `aws:kms`; KMS mode requires key ID |
| Egress identity | `worker_id`, `heartbeat_interval_ms`, `health_port` | `egress-1`, 5000, 8090; non-empty/positive valid port |
| Egress capabilities | `allowed_pools`, `tags`, `countries`, `regions`, `ip_types`, `supported_ingress_modes`, `max_concurrency` | `allowed_pools` defaults to `[{"deployment_id":"default","pool_id":"default"}]`; other lists are empty except ingress `rest`, `http_proxy`, `connect`; official workers advertise the built-in fingerprint catalogue; concurrency defaults to 4 at worker composition |
| connection pool | `enabled`, `max_idle_conns_per_host`, `idle_timeout_ms`, `max_lifetime_ms` | false; when enabled 8, 30000, 300000 |
| HTTP/2 | `enabled`, `fallback_cache_ttl_ms` | false, 300000 |
| NATS | `servers`, `user_credentials_file`, `username_env`, `password_env` | `nats://127.0.0.1:4222`, empty; credential file or named user/password environment variables |
| NATS liveness | `reconnect_attempts`, `reconnect_wait_ms`, `ping_interval_ms`, `max_ping_failures`, `max_payload_bytes` | 10, 2000, 30000, 3, server-discovered; payload must fit configured frames/inline limits |

## Complete field index

The following names are the normative index for runtime snapshot and optional-profile fields. Defaults are produced
by the validated configuration loader; omitted optional values retain those defaults. Every change requires restart
unless it is part of a runtime snapshot activated through the Admin API.

| Object | Fields and constraints |
| --- | --- |
| routing rule | `routing_rules`, `id`, `priority`, `enabled`, `match`, `target_pool_id`, `sticky_session_ttl_seconds`, `allow_sticky_fallback`; priorities define ordering, referenced pools must exist, and sticky fallback requires a positive TTL |
| match | `tags`, `country`, `region`, `ip_type`, `ingress_type`, `target_host`; omitted members do not restrict the match |
| executor pool | `executor_pools`, `id`, `enabled`, `executor_type`, `tags`, `allow_degraded_workers`, `allowed_ip_types`, `allowed_countries`, `allowed_regions`; pool IDs are unique; disabled pools receive no new assignments; executor type and tags are hard worker-eligibility constraints; non-empty allowed lists bound the worker's advertised capabilities |
| destination rule | `destination_policy`, `rule_type`, `action`, `reason`, `raw_pattern`, `normalized_host`, `normalized_cidr`, `normalized_ip`, `normalized_name`; raw patterns are authoritative, normalized fields are server output, and malformed rules are rejected before activation |
| injection policy | `injection_policies`, `operations`, `op`, `header_name`, `value_base64`; header operations preserve declared order and reject invalid names/values |
| fingerprint profile | `fingerprint_profiles`, `name`, `scope_type`, `supported_by_worker`, `executor_type`, `profile_ref`, `contract_revision`; activation requires worker support |
| worker setting | `worker_settings`, `worker_id`, `enabled`, `draining`; lifecycle changes are deployment-scoped |
| snapshot | `config_version`, `default_timeout_ms`, `max_timeout_ms`; versions increase and timeout bounds must be positive and ordered |
| Egress capabilities | `allowed_pools`, `countries`, `regions`, `ip_types`, `supported_ingress_modes`; values describe pool membership and admission capabilities rather than network authorization; omitted pool membership is `default/default`; duplicate, unknown, or non-default deployment references are rejected |
| upstream connection pool | `max_idle_conns_per_host`, `idle_timeout_ms`, `max_lifetime_ms`; zero/negative invalid values are rejected by config validation |
| object storage | `local_directory`, `max_part_bytes`, `assignment_ttl_seconds`, `cleanup_interval_seconds`, `server_side_encryption`; local storage is development-only and production encryption/retention are operator responsibilities |

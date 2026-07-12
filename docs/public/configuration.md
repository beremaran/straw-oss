---
sidebar_position: 4
---

# Configuration

Control and Egress accept `-config PATH` with a strict, versioned JSON file. Unknown fields are rejected so typos do
not silently change behavior. Without `-config`, both binaries use local defaults.

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
    "runtime_admin": {"enabled": false}
  }
}
```

Set `STRAW_AUTH_TOKEN` to require `Authorization: Bearer <token>` on requests. An unset token permits requests and is
appropriate only for a loopback or otherwise trusted development network.

### Optional runtime administration

`runtime_admin.enabled` opts into durable runtime configuration. Its defaults are `token_env: "STRAW_ADMIN_TOKEN"`,
`bucket: "STRAW_RUNTIME_CONFIG"`, and `history_limit: 64`. The named token environment variable must be non-empty,
and NATS must have JetStream file storage enabled. See [Runtime administration](runtime-administration.md).

### Optional shared runtime state

`runtime_state.backend` defaults to `memory`. Set it to `redis` only when multiple Control instances must be
interchangeable. Redis credentials belong in the URL named by `redis_url_env` (default `STRAW_REDIS_URL`); both
`redis://` and TLS-protected `rediss://` URLs are accepted.

```json
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
```

`request_ttl_ms` must exceed `request.max_timeout_ms`. Instance IDs must be unique NATS subject tokens; if the named
environment variable is empty, Control generates a process-unique ID. See [Highly available Control](highly-available-control.md)
for TTL and outage behavior.

## Egress

```json
{
  "config_version": "v1",
  "egress": {
    "worker_id": "egress-1",
    "heartbeat_interval_ms": 5000,
    "health_port": 8090,
    "capabilities": {"max_concurrency": 4},
    "nats": {"servers": ["nats://127.0.0.1:4222"]},
    "upstream_connection_pool": {"enabled": false},
    "http2": {"enabled": false, "fallback_cache_ttl_ms": 300000}
  }
}
```

Worker IDs must be unique within a deployment. `max_concurrency` defaults to `4`. The optional connection pool can
reuse upstream connections; when enabled its defaults are 8 idle connections per deployment/host, 30 seconds idle
timeout, and 5 minutes maximum lifetime.

## NATS authentication

Both services support:

- `user_credentials_file`: path to a NATS credentials file; or
- `username_env` and `password_env`: names of environment variables containing credentials.

The production example uses `STRAW_NATS_USER` and `STRAW_NATS_PASSWORD`. Never put secret values directly in the JSON
files. Connection tuning fields are `reconnect_attempts`, `reconnect_wait_ms`, `ping_interval_ms`,
`max_ping_failures`, and `max_payload_bytes`.

`deploy/local/*.json` and `deploy/production/*.json` are the canonical working examples.

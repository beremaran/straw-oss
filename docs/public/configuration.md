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

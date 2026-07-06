## 24. Static Configuration

All config files include top-level `config_version`.

### Canonical Config Key Paths

The following paths are the canonical config keys used throughout the system. All references in other sections must use
these full paths.

| Config Key                                              | Default    | Section(s) Referenced              |
|---------------------------------------------------------|------------|------------------------------------|
| `control.server.host`                                   | `0.0.0.0`  | 24                                 |
| `control.server.api_port`                               | `8080`     | 24                                 |
| `control.server.metrics_port`                           | `9090`     | 7, 24 (metrics, healthz, readyz)   |
| `control.server.read_timeout_ms`                        | `30000`    | 24                                 |
| `control.server.write_timeout_ms`                       | `30000`    | 24                                 |
| `control.request.default_timeout_ms`                    | `60000`    | 24                                 |
| `control.request.max_timeout_ms`                        | `300000`   | 24                                 |
| `control.request.max_inline_request_body_bytes`         | `1048576`  | 24                                 |
| `control.request.max_inline_response_body_bytes`        | `1048576`  | 24                                 |
| `control.worker.availability_timeout_ms`                | `15000`    | 11                                 |
| `control.worker.dead_timeout_ms`                        | `30000`    | 11                                 |
| `control.worker.duplicate_session_grace_ms`             | `10000`    | 11                                 |
| `control.worker.assignment_ack_timeout_ms`              | `2000`     | 11                                 |
| `control.worker.cooldown_failure_count`                 | `3`        | 11                                 |
| `control.worker.cooldown_window_ms`                     | `60000`    | 11                                 |
| `control.worker.cooldown_duration_ms`                   | `30000`    | 11                                 |
| `control.worker.registration_clock_skew_ms`             | `60000`    | 27                                 |
| `control.worker.registration_nonce_ttl_ms`              | `300000`   | 27                                 |
| `control.worker.registration_fail_open_on_redis_outage` | `false`    | 27                                 |
| `control.transport.max_frame_data_bytes`                | `1048576`  | 12                                 |
| `control.transport.initial_upload_credit_bytes`         | `8388608`  | 12                                 |
| `control.transport.initial_download_credit_bytes`       | `8388608`  | 12                                 |
| `control.transport.max_inflight_upload_bytes`           | `16777216` | 12                                 |
| `control.transport.max_inflight_download_bytes`         | `16777216` | 12                                 |
| `control.transport.frame_idle_timeout_ms`               | `15000`    | 12                                 |
| `control.nats.servers`                                  | —          | 24                                 |
| `control.nats.user_credentials_file`                    | —          | 24                                 |
| `control.nats.reconnect_attempts`                       | `10`       | 24                                 |
| `control.nats.reconnect_wait_ms`                        | `2000`     | 24                                 |
| `control.nats.ping_interval_ms`                         | `30000`    | 24                                 |
| `control.nats.max_ping_failures`                        | `3`        | 24                                 |
| `control.nats.max_payload_bytes`                        | `null`     | 24 (discovered from server if null)|
| `control.database.postgres.dsn_env`                     | —          | 24                                 |
| `control.database.postgres.max_open_conns`              | `20`       | 24                                 |
| `control.database.postgres.max_idle_conns`              | `5`        | 24                                 |
| `control.database.postgres.conn_max_lifetime_minutes`   | `30`       | 24                                 |
| `control.database.redis.url_env`                        | —          | 24                                 |
| `control.database.redis.max_open_conns`                 | `10`       | 24                                 |
| `control.database.redis.conn_max_lifetime_minutes`      | `10`       | 24                                 |
| `control.database.clickhouse.url`                       | —          | 24                                 |
| `control.database.clickhouse.database`                  | `straw`    | 24                                 |
| `control.database.clickhouse.username`                  | `straw`    | 24                                 |
| `control.database.clickhouse.password_env`              | —          | 24                                 |
| `control.database.clickhouse.max_conns`                 | `10`       | 24                                 |
| `control.database.clickhouse.async_write`               | `true`     | 24                                 |
| `control.database.clickhouse.write_batch_size`          | `1000`     | 24                                 |
| `control.database.clickhouse.write_flush_interval_ms`   | `1000`     | 24                                 |
| `control.database.clickhouse.write_queue_max_entries`   | `100000`   | 24                                 |
| `control.observability.logging.level`                   | `info`     | 24                                 |
| `control.observability.logging.format`                  | `json`     | 24                                 |
| `control.observability.logging.output`                  | `["stdout"]`| 24                                |
| `control.observability.metrics.enabled`                 | `true`     | 24                                 |
| `control.observability.metrics.path`                    | `/metrics` | 24                                 |
| `control.observability.tracing.enabled`                 | `false`    | 24                                 |
| `control.observability.tracing.exporter`                | `jaeger`   | 24                                 |
| `control.observability.tracing.endpoint`                | —          | 24                                 |
| `control.observability.tracing.sampling_rate`           | `0.1`      | 24                                 |
| `control.observability.tracing.propagate_trace_context` | `true`     | 24                                 |
| `egress.worker_id`                                      | —          | 24                                 |
| `egress.nats.servers`                                   | —          | 24                                 |
| `egress.nats.user_credentials_file`                     | —          | 24                                 |
| `egress.nats.reconnect_attempts`                        | `10`       | 24                                 |
| `egress.nats.reconnect_wait_ms`                         | `2000`     | 24                                 |
| `egress.nats.ping_interval_ms`                          | `30000`    | 24                                 |
| `egress.nats.max_ping_failures`                         | `3`        | 24                                 |
| `egress.credential.credential_id_env`                   | —          | 24                                 |
| `egress.credential.private_key_env`                     | —          | 24                                 |
| `egress.private_key_ed25519_env`                        | —          | 24, 27 (implemented flat key, see note below the tables) |
| `egress.capabilities.pool_ids`                          | —          | 24                                 |
| `egress.capabilities.tags`                              | `[]`       | 24                                 |
| `egress.capabilities.countries`                         | `[]`       | 24                                 |
| `egress.capabilities.regions`                           | `[]`       | 24                                 |
| `egress.capabilities.ip_types`                          | `[]`       | 24                                 |
| `egress.capabilities.supported_ingress_modes`           | `["rest"]` | 24                                 |
| `egress.capabilities.max_concurrency`                   | —          | 24                                 |
| `egress.heartbeat.interval_ms`                          | `5000`     | 11                                 |
| `egress.outbound_tls.strict_verify`                     | `true`     | 24                                 |
| `egress.outbound_tls.ca_bundle_path`                    | —          | 24                                 |
| `egress.outbound_proxy.enabled`                         | `false`    | 24                                 |
| `egress.outbound_proxy.type`                            | `http`     | 24                                 |
| `egress.outbound_proxy.host`                            | —          | 24                                 |
| `egress.outbound_proxy.port`                            | —          | 24                                 |
| `egress.outbound_proxy.username_env`                    | —          | 24                                 |
| `egress.outbound_proxy.password_env`                    | —          | 24                                 |
| `egress.dns.mode`                                      | `system`   | 24                                 |
| `egress.dns.custom_servers`                             | `[]`       | 24                                 |
| `egress.connect_timeout_ms`                              | `10000`    | 16, 24                             |
| `egress.response_header_timeout_ms`                      | `30000`    | 16, 24                             |
| `egress.upload_idle_timeout_ms`                          | `30000`    | 16, 24                             |
| `egress.download_idle_timeout_ms`                        | `30000`    | 16, 24                             |
| `egress.observability.logging.level`                     | `info`     | 24                                 |
| `egress.observability.logging.format`                    | `json`     | 24                                 |
| `egress.observability.logging.output`                    | `["stdout"]`| 24                                |
| `egress.observability.health.enabled`                    | `true`     | 24                                 |
| `egress.observability.health.host`                       | `0.0.0.0`  | 24                                 |
| `egress.observability.health.port`                       | `9090`     | 24                                 |

`egress.worker_id` and `egress.credential_id` are implemented as flat top-level keys (not nested under
`egress.credential`), a pre-existing gap predating task 35. `egress.private_key_ed25519_env` (added by task 35) is
likewise flat, alongside them, rather than `egress.credential.private_key_env`, for consistency with the fields it
sits next to. Reconciling the nested `egress.credential.*` shape into the implemented flat shape is owned by
`docs/tasks/p1/22-egress-credential-config-schema-reconciliation.md`.

### Control Config Example

```yaml
config_version: "v1"
control:
  server:
    host: "0.0.0.0"
    api_port: 8080
    metrics_port: 9090
    read_timeout_ms: 30000
    write_timeout_ms: 30000

  request:
    default_timeout_ms: 60000
    max_timeout_ms: 300000
    max_inline_request_body_bytes: 1048576
    max_inline_response_body_bytes: 1048576

  worker:
    availability_timeout_ms: 15000
    dead_timeout_ms: 30000
    duplicate_session_grace_ms: 10000
    assignment_ack_timeout_ms: 2000
    cooldown_failure_count: 3
    cooldown_window_ms: 60000
    cooldown_duration_ms: 30000
    registration_clock_skew_ms: 60000
    registration_nonce_ttl_ms: 300000
    registration_fail_open_on_redis_outage: false

  transport:
    max_frame_data_bytes: 1048576
    initial_upload_credit_bytes: 8388608
    initial_download_credit_bytes: 8388608
    max_inflight_upload_bytes: 16777216
    max_inflight_download_bytes: 16777216
    frame_idle_timeout_ms: 15000

  nats:
    servers: [ "nats://nats:4222" ]
    user_credentials_file: "/etc/straw/nats/control.creds"
    reconnect_attempts: 10
    reconnect_wait_ms: 2000
    ping_interval_ms: 30000
    max_ping_failures: 3
    max_payload_bytes: null

  database:
    postgres:
      dsn_env: "STRAW_POSTGRES_DSN"
      max_open_conns: 20
      max_idle_conns: 5
      conn_max_lifetime_minutes: 30
    redis:
      url_env: "STRAW_REDIS_URL"
      max_open_conns: 10
      conn_max_lifetime_minutes: 10
    clickhouse:
      url: "http://clickhouse:8123"
      database: "straw"
      username: "straw"
      password_env: "STRAW_CLICKHOUSE_PASSWORD"
      max_conns: 10
      async_write: true
      write_batch_size: 1000
      write_flush_interval_ms: 1000
      write_queue_max_entries: 100000

  observability:
    logging:
      level: "info"
      format: "json"
      output: [ "stdout" ]
    metrics:
      enabled: true
      path: "/metrics"
    tracing:
      enabled: false
      exporter: "jaeger"
      endpoint: "http://jaeger:14268/api/traces"
      sampling_rate: 0.1
      propagate_trace_context: true

# P2-only body_transport. Ignored or rejected in P0 unless feature flag enabled.
# body_transport:
#   large_body_threshold_bytes: 1048576
#   object_storage:
#     enabled: false
#     endpoint: "https://s3.amazonaws.com"
#     bucket: "straw-bodies"
#     region: "us-east-1"
#     access_key_env: "STRAW_S3_ACCESS_KEY"
#     secret_key_env: "STRAW_S3_SECRET_KEY"
#     body_retention_days: 1
#   direct_stream:
#     enabled: false
#     endpoint: "http://body-stream:9090"
#     stream_timeout_ms: 300000
```

### Egress Config Example

```yaml
config_version: "v1"
egress:
  worker_id: "egress-local-001"
  credential_id: "wcred-local-001"
  private_key_ed25519_env: "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64"

  nats:
    servers: [ "nats://nats:4222" ]
    user_credentials_file: "/etc/straw/nats/egress.creds"
    reconnect_attempts: 10
    reconnect_wait_ms: 2000
    ping_interval_ms: 30000
    max_ping_failures: 3

  credential:
    credential_id_env: "STRAW_WORKER_CREDENTIAL_ID"
    private_key_env: "STRAW_WORKER_PRIVATE_KEY"

  capabilities:
    pool_ids: [ "default" ]
    tags: [ "datacenter", "local" ]
    countries: [ "AU" ]
    regions: [ "wa" ]
    ip_types: [ "datacenter" ]
    supported_ingress_modes: [ "rest" ]
    max_concurrency: 100

  heartbeat:
    interval_ms: 5000

  outbound_tls:
    strict_verify: true
    ca_bundle_path: "/etc/straw/tls/ca-bundle.crt"

  outbound_proxy:
    enabled: false
    type: "http"
    host: "proxy.example.com"
    port: 8080
    username_env: "STRAW_UPSTREAM_PROXY_USERNAME"
    password_env: "STRAW_UPSTREAM_PROXY_PASSWORD"

  dns:
    mode: "system"
    custom_servers: [ ]

  connect_timeout_ms: 10000
  response_header_timeout_ms: 30000
  upload_idle_timeout_ms: 30000
  download_idle_timeout_ms: 30000

  observability:
    logging:
      level: "info"
      format: "json"
      output: [ "stdout" ]
    health:
      enabled: true
      host: "0.0.0.0"
      port: 9090
```

### Environment Variable Corrections

All variables use `STRAW_`, never `STROW_`.

Canonical examples:

- `STRAW_MITM_CERT_VALIDITY_DAYS`,
- `STRAW_BODY_OBJECT_STORAGE_ENABLED`,
- `STRAW_UPSTREAM_PROXY_USERNAME`,
- `STRAW_UPSTREAM_PROXY_PASSWORD`,
- `STRAW_BODY_RETENTION_DAYS`.

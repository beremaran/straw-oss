## 24. Static Configuration

All config files include top-level `config_version`.

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

  body_transport:
    large_body_threshold_bytes: 1048576
    object_storage:
      enabled: false
      endpoint: "https://s3.amazonaws.com"
      bucket: "straw-bodies"
      region: "us-east-1"
      access_key_env: "STRAW_S3_ACCESS_KEY"
      secret_key_env: "STRAW_S3_SECRET_KEY"
      body_retention_days: 1
    direct_stream:
      enabled: false
      endpoint: "http://body-stream:9090"
      stream_timeout_ms: 300000

  observability:
    logging:
      level: "info"
      format: "json"
      output: [ "stdout" ]
    metrics:
      enabled: true
      path: "/metrics"
      host: "0.0.0.0"
      port: 9090
    tracing:
      enabled: false
      exporter: "jaeger"
      endpoint: "http://jaeger:14268/api/traces"
      sampling_rate: 0.1
      propagate_trace_context: true
```

### Egress Config Example

```yaml
config_version: "v1"
egress:
  worker_id: "egress-local-001"

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

  upstream_proxy:
    enabled: false
    type: "http"
    host: "proxy.example.com"
    port: 8080
    username_env: "STRAW_UPSTREAM_PROXY_USERNAME"
    password_env: "STRAW_UPSTREAM_PROXY_PASSWORD"

  dns:
    mode: "system"
    custom_servers: [ ]

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

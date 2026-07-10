## 22. Canonical ClickHouse Schema

This section is canonical. Observability prose must refer here rather than defining separate schemas.

Control writes asynchronously through a bounded queue. If the queue fills, oldest non-critical events are dropped and
metrics/alerts fire. Request transport does not block on ClickHouse.

### Database Layout

Use one ClickHouse database by default: `straw`. Tables are namespaced by table name, not separate databases, unless
operators intentionally split databases.

### `request_events`

```sql
CREATE TABLE straw.request_events
(
    timestamp           DateTime64(3, 'UTC'),
    request_id          String,
    trace_id            String,
    tenant_id           LowCardinality(String),
    api_key_id          String,
    ingress_type        LowCardinality(String),
    method              LowCardinality(String),
    target_host         String,
    target_url          String,
    route_id            String,
    pool_id             String,
    executor_type       LowCardinality(String),
    selected_executor   String,
    requested_fingerprint_profile LowCardinality(String) DEFAULT '',
    selected_fingerprint_profile  LowCardinality(String) DEFAULT '',
    executed_fingerprint_profile  LowCardinality(String) DEFAULT '',
    country             LowCardinality(String),
    region              LowCardinality(String),
    ip_type             LowCardinality(String),
    tags                Array(String),
    attempt             UInt8,
    upstream_status     UInt16,
    client_status       UInt16,
    error_code          LowCardinality(String),
    error_category      LowCardinality(String),
    timeout_type        LowCardinality(String),
    request_size_bytes  UInt64,
    response_size_bytes UInt64,
    routing_ms          UInt32,
    assignment_ms       UInt32,
    egress_ms           UInt32,
    total_ms            UInt32,
    capture_decision    LowCardinality(String)
) ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, timestamp, request_id)
TTL timestamp + INTERVAL 90 DAY;
```

Fingerprint evidence is additive and redacted: safe catalog/default tokens are stored literally; unsafe submitted
values are projected as `sha256:<16 hex>:len=<bytes>`. `executed_fingerprint_profile` is empty when Egress was never
reached, including unsupported-profile rejection. Existing volumes must receive these columns through the idempotent
ClickHouse migration before the schema check is considered complete.

### `worker_events`

```sql
CREATE TABLE straw.worker_events
(
    timestamp          DateTime64(3, 'UTC'),
    tenant_id          LowCardinality(String),
    worker_id          String,
    session_id         String,
    executor_type      LowCardinality(String),
    event_type         LowCardinality(String),
    health             LowCardinality(String),
    active_requests    UInt32,
    max_concurrency    UInt32,
    available_capacity UInt32,
    draining           UInt8,
    reason             String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, worker_id, timestamp)
TTL timestamp + INTERVAL 90 DAY;
```

### `config_audit_events`

```sql
CREATE TABLE straw.config_audit_events
(
    timestamp      DateTime64(3, 'UTC'),
    tenant_id      LowCardinality(String),
    actor_type     LowCardinality(String),
    actor_id       String,
    config_type    LowCardinality(String),
    resource_id    String,
    action         LowCardinality(String),
    config_version UInt64,
    field_path     String,
    old_value_json String,
    new_value_json String
) ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (tenant_id, timestamp, config_type, resource_id)
TTL timestamp + INTERVAL 180 DAY;
```

**Secret redaction**: Before writing `old_value_json` and `new_value_json`, Control classifies each field per the
secret classification rules in Section 21. Fields classified as `secret` are replaced with `[redacted]`. Fields
classified as `sensitive` are stored as a hash or bounded metadata only. This redaction applies to both Postgres
`config_audit_source` records and ClickHouse `config_audit_events`.

### `log_events`

```sql
CREATE TABLE straw.log_events
(
    timestamp  DateTime64(3, 'UTC'),
    service    LowCardinality(String),
    level      LowCardinality(String),
    message    String,
    request_id String,
    tenant_id  String,
    trace_id   String,
    worker_id  String,
    error_code LowCardinality(String),
    extra      Map(String, String)
) ENGINE = MergeTree
PARTITION BY toYYYYMM(timestamp)
ORDER BY (service, timestamp)
TTL timestamp + INTERVAL 30 DAY;
```

### `payload_capture_events` — P2

```sql
CREATE TABLE straw.payload_capture_events
(
    captured_at       DateTime64(3, 'UTC'),
    request_id        String,
    tenant_id         LowCardinality(String),
    capture_scope     LowCardinality(String),
    capture_decision  LowCardinality(String),
    request_headers   String,
    response_headers  String,
    request_body_ref  String,
    response_body_ref String,
    redacted_fields   Array(String),
    truncated         UInt8
) ENGINE = MergeTree
PARTITION BY toYYYYMM(captured_at)
ORDER BY (tenant_id, captured_at, request_id)
TTL captured_at + INTERVAL 7 DAY;
```

### Telemetry Exposure

Internal telemetry tables may store `worker_id`, `session_id`, and `selected_executor`. Tenant-facing telemetry APIs
must either omit these fields or return stable public aliases that do not reveal internal topology. See Section 21 for
full telemetry exposure rules.

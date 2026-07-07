-- ClickHouse schema for the Straw local stack.
-- Canonical source: docs/planning/22-canonical-clickhouse-schema.md.
-- Mounted at /docker-entrypoint-initdb.d so ClickHouse applies it on first boot.

CREATE DATABASE IF NOT EXISTS straw;

CREATE TABLE IF NOT EXISTS straw.request_events
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
TTL toDateTime(timestamp) + INTERVAL 90 DAY;

CREATE TABLE IF NOT EXISTS straw.worker_events
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
TTL toDateTime(timestamp) + INTERVAL 90 DAY;

CREATE TABLE IF NOT EXISTS straw.config_audit_events
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
TTL toDateTime(timestamp) + INTERVAL 180 DAY;

CREATE TABLE IF NOT EXISTS straw.log_events
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
TTL toDateTime(timestamp) + INTERVAL 30 DAY;

-- P2: payload capture metadata. Bodies are stored in object storage by
-- reference (request_body_ref / response_body_ref); only redacted headers and
-- reference keys land here. The 7-day TTL is the retention backstop from
-- docs/planning/22.
CREATE TABLE IF NOT EXISTS straw.payload_capture_events
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
TTL toDateTime(captured_at) + INTERVAL 7 DAY;

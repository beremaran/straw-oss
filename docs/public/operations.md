# System Operations Guide

This guide details operational guidelines for maintaining, configuring, and monitoring a Straw production deployment.

---

## 1. Health & Readiness Probes

Both the Control Plane and Egress Worker binaries expose local HTTP health endpoints to assist with load balancer integration and orchestrator liveness/readiness checks.

### Control Plane Ports
Both `server.api_port` and `server.metrics_port` are required config fields with no built-in default — the Docker Compose stack in this repo sets them to `8080` and `9090` respectively, and this guide uses those values in its examples.
- **REST / Config API Port**: `8080` (Compose stack default)
- **HTTP Forward Proxy Port**: `8081` (default when `server.proxy_enabled` is true)
- **HTTP CONNECT Tunnel Port**: `8082` (default when `server.connect_enabled` is true)
- **MITM Inspection Proxy Port**: `8083` (default when `server.mitm_enabled` is true)
- **Metrics / Health Port**: `9090` (Compose stack default)

### Egress Worker Ports
- **Health Port**: `8090` (default)

### Endpoints
- **Liveness Probe (`GET /healthz`)**:
  - Unauthenticated.
  - Returns `200 OK` (with body `OK`) if the service is running.
- **Readiness Probe (`GET /readyz`)**:
  - Unauthenticated.
  - Returns `200 OK` (with body `OK`) when the service is healthy and accepting traffic.
  - During graceful shutdown (SIGINT/SIGTERM), `/readyz` immediately switches to return `503 Service Unavailable` so load balancers stop routing new traffic while active in-flight requests complete their drain period.

---

## 2. Prometheus Metrics

Straw's Control plane exposes a Prometheus scrape endpoint at `GET http://localhost:9090/metrics` (metrics server port).

The following metrics are exported:

### Request & Routing Metrics
- `straw_requests_total` (Counter): Total Control REST requests dispatched. Labeled by `tenant_id` and `error_code` (empty string on success).
- `straw_request_duration_seconds` (Histogram): End-to-end Control REST request execution time. Labeled by `tenant_id`.
- `straw_active_requests` (Gauge): Current count of concurrent requests in-flight on the Control plane.
- `straw_routing_duration_seconds` (Histogram): Time spent evaluating routing rules and locating eligible candidate pools.

### Transport & Scheduling Metrics
- `straw_assignment_duration_seconds` (Histogram): Total round-trip duration for worker scheduling.
- `straw_nats_request_duration_seconds` (Histogram): Round-trip duration for NATS assign-request/ack packets.
- `straw_nats_errors_total` (Counter): Count of NATS network or protocol errors. Labeled by `error_code`.

### Admission Control Metrics
- `straw_rate_limit_rejections_total` (Counter): Count of requests rejected due to tenant rate-limiting. Labeled by `tenant_id`.
- `straw_quota_rejections_total` (Counter): Count of requests rejected due to tenant bandwidth or request quota exhaustion. Labeled by `tenant_id`.

### Worker Registry Status
- `straw_worker_sessions` (Gauge): Number of Egress worker sessions currently registered with the Control plane.
- `straw_workers_available` (Gauge): Number of registered workers currently eligible for new request assignments.
- `straw_worker_heartbeat_age_seconds` (Gauge): Heartbeat age of the stalest active worker.

### Egress Metrics
Enabled only when Control config `server.egress_metrics_enabled` is `true`. These metrics are aggregated by Control
from worker heartbeats and are exposed only on Control's `/metrics`; Egress does not expose a Prometheus endpoint.
If Control cannot refresh Redis-backed worker runtime state during a scrape, it reports from its local snapshot.
- `straw_egress_active_requests` (Gauge): Aggregate active requests reported by Egress workers.
- `straw_egress_max_concurrency` (Gauge): Aggregate max concurrency reported by Egress workers.
- `straw_egress_available_capacity` (Gauge): Aggregate available capacity reported by Egress workers.

### Telemetry Pipeline Metrics
- `straw_clickhouse_write_queue_depth` (Gauge): Count of telemetry events buffered in memory waiting for a ClickHouse flush.
- `straw_clickhouse_write_errors_total` (Counter): Count of failed ClickHouse batch insert operations.

---

## 3. Environment Configuration

Durable secrets and database URLs are loaded strictly from environment variables:

| Variable | Description | Component | Required |
| :--- | :--- | :--- | :--- |
| `STRAW_POSTGRES_DSN` | Connection string for Postgres (e.g. `postgres://user:pass@host:5432/db`). | Control | Yes |
| `STRAW_REDIS_URL` | URL of the Redis instance (e.g. `redis://host:6379/0`). | Control | Yes |
| `STRAW_API_KEY_PEPPER` | Secret pepper used to hash client API keys. | Control | Yes |
| `STRAW_BOOTSTRAP_SYSTEM_ADMIN_KEY` | Bootstrap admin key secret. Creates the first `system_admin` platform key if the store is empty. | Control | No |
| `STRAW_CLICKHOUSE_USER` | Username for the ClickHouse HTTP interface. | Control | No |
| `STRAW_CLICKHOUSE_PASSWORD` | Password for the ClickHouse HTTP interface. | Control | No |
| `STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64` | Base64-encoded persistent Ed25519 private key (32-byte seed or 64-byte private key) used by the worker for registration signatures. | Egress | Yes |

---

## 4. Structured JSON Logging

Both Control and Egress output structured logs to `stdout` in JSON format.

### Log Fields
- `timestamp`: RFC3339 nano formatted timestamp (UTC).
- `level`: `INFO`, `WARN`, `ERROR`, or `DEBUG`.
- `msg`: The log message summary.
- `service`: Binary identifier (`control` or `egress`).
- `error`: Formatted error details (if present).
- Context-specific fields (e.g., `tenant_id`, `worker_id`, `request_id`).

---

## 5. Current Operational Limits & Fallbacks

Platform operators should be aware of the following current system limits and failure behaviors:

1. **REST Ingress Limits**:
   - Ingress request and response bodies must fit entirely in memory.
   - The default payload size limit is `1048576` bytes (1 MiB).
   - Upstream streaming responses are buffered in full by the Control plane before being returned to the REST client.
2. **NATS Message Payload Constraints**:
   - The NATS server `max_payload` configuration must comfortably accommodate the Control plane's `max_frame_data_bytes` and inline body limits. The stock 1 MiB NATS default will fail validation checks on boot unless increased to `2MB` or higher.
3. **Redis Outages**:
   - Redis is used for tenant rate-limiting, sticky session maps, and config cache invalidation pub/sub.
   - If Redis is unavailable at startup, the Control plane issues warning logs and continues booting.
   - During runtime Redis outages, components degrade gracefully (e.g., rate limits and quotas fail open or closed depending on configuration, and cache invalidation falls back to periodic 30-second Postgres polling).
4. **ClickHouse Telemetry Outages**:
   - Egress request metadata events are buffered in memory and flushed in batches.
   - If ClickHouse experiences an outage, the queue will buffer events up to `max_queue_entries` (default `10000`). Once the queue is full, Straw drops oldest telemetry events to prevent memory exhaustion and preserve request-forwarding availability.

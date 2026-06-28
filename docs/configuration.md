# Configuration Reference

Straw Proxy is configured entirely using environment variables. This section contains the complete configuration references for both the Relay Server and the Endpoint Worker.

---

## 🔒 Shared Security Requirement

> [!IMPORTANT]
> The `HMAC_SECRET` variable **must be identical** on the Relay Server and all running Endpoint Workers. The Relay Server signs outgoing task payloads using this secret, and workers will reject any tasks with invalid signatures.

---

## 📡 Relay Server Configuration

These variables configure the behavior of the orchestrator gateway (`cmd/relay-server`).

### Required Variables

| Variable | Type | Description |
|---|---|---|
| `POSTGRES_DSN` | string | PostgreSQL database connection string (DSN). Example: `postgres://postgres:pass@localhost:5432/db` |
| `NATS_URL` | string | NATS message broker URL. Example: `nats://localhost:4222` |
| `HMAC_SECRET` | string | Secret key used for signing task payloads and verifying results. |

### Operational Defaults

| Variable | Type | Default | Description |
|---|---|---|---|
| `DB_AUTO_MIGRATE` | boolean | `false` | If `true`, runs embedded SQL migrations on startup. |
| `HTTP_PORT` | integer | `8080` | Port for the client-facing proxy API. |
| `ADMIN_PORT` | integer | `8081` | Port for the administrative API. |
| `ADMIN_API_KEY` | string | *(empty)* | Optional API key required to access the Admin API (via Bearer token header). |
| `SHUTDOWN_TIMEOUT` | duration | `30s` | Time allocated for graceful shutdown of active HTTP clients. |
| `RESULT_TIMEOUT` | duration | `30s` | Maximum time to wait for a worker to return results before timing out. |
| `MAX_BODY_SIZE` | string | `2M` | Maximum allowed request body size (e.g. `500K`, `2M`, `10M`). |
| `MAX_CONCURRENT_REQUESTS` | integer | `50` | Maximum limit of client requests handled simultaneously. |
| `ALLOW_PRIVATE_IPS` | boolean | `false` | If `true`, disables SSRF protection and allows forwarding requests to local subnet IPs (useful for local development). |

### Redis Settings

| Variable | Type | Default | Description |
|---|---|---|---|
| `REDIS_ADDR` | string | `localhost:6379` | Redis server address. |
| `REDIS_POOL_SIZE` | integer | `100` | Redis connection pool size limit. |
| `REDIS_MIN_IDLE_CONNS` | integer | `10` | Minimum idle connections kept in the Redis pool. |

### Message Broker (NATS) Settings

| Variable | Type | Default | Description |
|---|---|---|---|
| `NATS_TOKEN` | string | *(empty)* | Authentication token if NATS is secured. |

### Security & TLS

| Variable | Type | Default | Description |
|---|---|---|---|
| `TLS_CERT_FILE` | string | *(empty)* | Path to SSL/TLS certificate file (enables HTTPS on the client API). |
| `TLS_KEY_FILE` | string | *(empty)* | Path to private SSL/TLS key file. |
| `VAULT_ADDR` | string | *(empty)* | Optional HashiCorp Vault server address for secure secrets retrieval. |

### Observability & Telemetry

| Variable | Type | Default | Description |
|---|---|---|---|
| `LOG_LEVEL` | string | `info` | Logging verbosity: `debug`, `info`, `warn`, `error`. |
| `LOG_FORMAT` | string | `json` | Logging format: `json` or `console`. |
| `METRICS_ENABLED` | boolean | `true` | Exposes a Prometheus metrics server. |
| `METRICS_PORT` | integer | `9090` | Port for the metrics server (serves `/metrics` and `/debug/pprof`). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | string | *(empty)* | OTLP gRPC endpoint for trace exports (e.g., `localhost:4317`). |

---

## ⚙️ Endpoint Worker Configuration

These variables configure the execution daemon (`cmd/endpoint`).

### Required Variables

| Variable | Type | Description |
|---|---|---|
| `ENDPOINT_ID` | string | Unique identifier for this worker instance (e.g. `worker-eu-05`). |
| `NATS_URL` | string | NATS message broker URL. Must match the relay server. |
| `HMAC_SECRET` | string | Secret key used to verify task payload signatures and sign outgoing responses. |

### Egress & Processing Defaults

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENDPOINT_TAGS` | string | *(empty)* | Comma-separated tags defining worker capabilities (e.g. `type:residential,region:us`). Used for routing decisions. |
| `CONCURRENCY_LIMIT` | integer | `25` | Max number of HTTP request tasks processed concurrently by this worker. |
| `MAX_POOL_HOSTS` | integer | `1000` | Maximum number of idle hosts maintained in the HTTP client connection pool. |
| `IDLE_CONNS_PER_HOST` | integer | `10` | Maximum idle connections kept per host in the connection pool. |
| `IDLE_CONN_TIMEOUT` | duration | `90s` | Max duration an idle connection is kept open. |

### Automatic Updates

| Variable | Type | Default | Description |
|---|---|---|---|
| `SELF_UPDATE_ENABLED` | boolean | `true` | Enables/disables auto-updates of the worker binary. |
| `SELF_UPDATE_URL` | string | *(empty)* | URL to query for new version manifests. |
| `SELF_UPDATE_INTERVAL` | duration | `5m` | Frequency at which the updater queries the version manifest URL. |

### Message Broker (NATS) Settings

| Variable | Type | Default | Description |
|---|---|---|---|
| `NATS_TOKEN` | string | *(empty)* | Authentication token if NATS is secured. |

### Security & TLS

| Variable | Type | Default | Description |
|---|---|---|---|
| `TLS_CERT_FILE` | string | *(empty)* | Path to certificate file. |
| `TLS_KEY_FILE` | string | *(empty)* | Path to private key file. |
| `VAULT_ADDR` | string | *(empty)* | HashiCorp Vault address. |

### Observability & Telemetry

| Variable | Type | Default | Description |
|---|---|---|---|
| `LOG_LEVEL` | string | `info` | Logging verbosity: `debug`, `info`, `warn`, `error`. |
| `LOG_FORMAT` | string | `json` | Logging format: `json` or `console`. |
| `METRICS_ENABLED` | boolean | `true` | Exposes a local health/metrics server. |
| `METRICS_PORT` | integer | `9090` | Port for the local health check server (serves `/healthz` and `/metrics`). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | string | *(empty)* | OTLP gRPC endpoint for trace exports. |

---

## 🌐 Shared / Global Settings

| Variable | Type | Default | Description |
|---|---|---|---|
| `OTEL_SDK_DISABLED` | boolean | `false` | Set to `true` to completely bypass trace generation on both the Relay and Worker. |

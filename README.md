# straw-proxy

## Environment Variables

### Relay Server (`cmd/relay-server/`)

#### Required

| Variable | Type | Description |
|---|---|---|
| `POSTGRES_DSN` | string | Postgres connection string (DSN). |
| `NATS_URL` | string | NATS message broker URL. |
| `HMAC_SECRET` | string | Secret used for HMAC request signing/verification. |

#### Optional

| Variable | Type | Default | Description |
|---|---|---|---|
| `DB_AUTO_MIGRATE` | bool | `false` | Run embedded database migrations on startup. |
| `HTTP_PORT` | int | `8080` | Main HTTP server port. |
| `ADMIN_PORT` | int | `8081` | Admin API server port. |
| `SHUTDOWN_TIMEOUT` | duration | `30s` | Graceful shutdown timeout. |
| `ADMIN_API_KEY` | string | *(empty)* | API key for authenticating admin API requests. |
| `RESULT_TIMEOUT` | duration | `30s` | Timeout waiting for endpoint task results. |
| `MAX_BODY_SIZE` | string | `2M` | Maximum request body size (e.g. `1M`, `10M`, `100K`). |
| `MAX_CONCURRENT_REQUESTS` | int | `50` | Maximum concurrent in-flight requests. |
| `ALLOW_PRIVATE_IPS` | bool | `false` | Allow forwarding requests to private IP addresses (SSRF protection bypass, testing mode). |

#### Redis

| Variable | Type | Default | Description |
|---|---|---|---|
| `REDIS_ADDR` | string | `localhost:6379` | Redis server address. |
| `REDIS_POOL_SIZE` | int | `100` | Redis connection pool size. |
| `REDIS_MIN_IDLE_CONNS` | int | `10` | Minimum idle connections in the Redis pool. |

#### NATS

| Variable | Type | Default | Description |
|---|---|---|---|
| `NATS_TOKEN` | string | *(empty)* | NATS authentication token. |

#### Security

| Variable | Type | Default | Description |
|---|---|---|---|
| `TLS_CERT_FILE` | string | *(empty)* | Path to TLS certificate file (for HTTPS). |
| `TLS_KEY_FILE` | string | *(empty)* | Path to TLS private key file (for HTTPS). |
| `VAULT_ADDR` | string | *(empty)* | HashiCorp Vault address (for secrets management). |

#### Observability

| Variable | Type | Default | Description |
|---|---|---|---|
| `LOG_LEVEL` | string | `info` | Log level (`debug`, `info`, `warn`, `error`). |
| `LOG_FORMAT` | string | `json` | Log format (`json`, `console`). |
| `METRICS_ENABLED` | bool | `true` | Enable the standalone metrics server. |
| `METRICS_PORT` | int | `9090` | Port for the metrics server (serves `/metrics` and pprof). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | string | *(empty)* | OTLP gRPC endpoint for OpenTelemetry trace export. |

---

### Endpoint Worker (`cmd/endpoint/`)

#### Required

| Variable | Type | Description |
|---|---|---|
| `ENDPOINT_ID` | string | Unique identifier for this endpoint instance. |
| `NATS_URL` | string | NATS message broker URL. |
| `HMAC_SECRET` | string | Secret used for HMAC request signing/verification (must match relay server). |

#### Optional

| Variable | Type | Default | Description |
|---|---|---|---|
| `ENDPOINT_TAGS` | string | *(empty)* | Comma-separated tags for this endpoint (e.g. `"us-east,high-memory"`). Used by the router to select endpoints. |
| `CONCURRENCY_LIMIT` | int | `25` | Maximum concurrent tasks processed by this endpoint. |
| `SELF_UPDATE_URL` | string | *(empty)* | URL to fetch self-update version manifests. |
| `SELF_UPDATE_INTERVAL` | duration | `5m` | How often to check for new versions. |
| `SELF_UPDATE_ENABLED` | bool | `true` | Enable automatic self-updates. |
| `MAX_POOL_HOSTS` | int | `1000` | Maximum number of hosts in the HTTP connection pool. |
| `IDLE_CONNS_PER_HOST` | int | `10` | Idle connections kept per host in the HTTP pool. |
| `IDLE_CONN_TIMEOUT` | duration | `90s` | Timeout for idle connections in the HTTP pool. |

#### NATS

| Variable | Type | Default | Description |
|---|---|---|---|
| `NATS_TOKEN` | string | *(empty)* | NATS authentication token. |

#### Security

| Variable | Type | Default | Description |
|---|---|---|---|
| `TLS_CERT_FILE` | string | *(empty)* | Path to TLS certificate file. |
| `TLS_KEY_FILE` | string | *(empty)* | Path to TLS private key file. |
| `VAULT_ADDR` | string | *(empty)* | HashiCorp Vault address. |

#### Observability

| Variable | Type | Default | Description |
|---|---|---|---|
| `LOG_LEVEL` | string | `info` | Log level (`debug`, `info`, `warn`, `error`). |
| `LOG_FORMAT` | string | `json` | Log format (`json`, `console`). |
| `METRICS_ENABLED` | bool | `true` | Enable the health/metrics server. |
| `METRICS_PORT` | int | `9090` | Port for the health/metrics server (serves `/healthz`, `/metrics`, and pprof). |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | string | *(empty)* | OTLP gRPC endpoint for OpenTelemetry trace export. |

---

### Shared / Global

| Variable | Type | Default | Description |
|---|---|---|---|
| `OTEL_SDK_DISABLED` | string | *(empty)* | Set to `true` to disable the OpenTelemetry SDK entirely (both relay server and endpoint). |

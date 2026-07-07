# Egress Worker Setup & Configuration Guide

Straw Egress Workers (`egress`) are lightweight, isolated execution daemons responsible for performing outbound HTTP/HTTPS calls on behalf of the Control plane. Workers register dynamically over NATS, accept request dispatches, apply TLS fingerprinting profiles, inject required headers, and stream response bodies back to the Control plane.

---

## How Workers Work

1. **Authentication**: Every worker is configured with a persistent Ed25519 private key. At startup, it signs a registration request containing its capabilities and target executor pools.
2. **Registration**: The registration request is sent to the Control plane via NATS. The Control plane verifies the signature using the pre-registered Ed25519 public key stored in Postgres.
3. **Replay Protection**: Control matches the registration nonce against Redis to prevent replay attacks.
4. **Execution Loop**: Once registered, the worker listens on a dedicated NATS subject for request assignments. It executes incoming requests concurrently up to its configured `max_concurrency` limit.

---

## Running the Egress Worker

The worker is run by passing a JSON configuration file:

```bash
./egress -config /etc/straw/egress.json
```

### Container Health Probes
The worker binary supports a `-healthcheck` flag designed for container environments (e.g. Docker `HEALTHCHECK` or Kubernetes probes). When run with this flag, it probes the worker's own local health endpoint and exits:

```bash
# Returns exit code 0 if healthy, or 1 if unhealthy
./egress -config /etc/straw/egress.json -healthcheck
```

---

## Configuration Schema (`egress.json`)

The config file uses a `v1` schema envelope containing a nested `egress` configuration block:

```json
{
  "config_version": "v1",
  "egress": {
    "worker_id": "egress-us-west-01",
    "credential_id": "11111111-1111-4111-8111-111111111111",
    "private_key_ed25519_env": "STRAW_WORKER_PRIVATE_KEY_ED25519_BASE64",
    "heartbeat_interval_ms": 5000,
    "health_port": 8090,
    "nats": {
      "servers": ["nats://nats-cluster.internal:4222"]
    },
    "allowed_pools": [
      {
        "tenant_id": "22222222-2222-4222-8222-222222222222",
        "pool_id": "default-pool"
      }
    ],
    "capabilities": {
      "supported_ingress_modes": ["rest", "mitm"],
      "ip_types": ["datacenter"],
      "countries": ["US"],
      "regions": ["us-west-1"],
      "tags": ["high-bandwidth"],
      "max_concurrency": 16
    },
    "upstream_connection_pool": {
      "enabled": true,
      "max_idle_conns_per_tenant_host": 8,
      "idle_timeout_ms": 60000,
      "max_lifetime_ms": 300000
    },
    "http2": {
      "enabled": true,
      "fallback_cache_ttl_ms": 300000
    }
  }
}
```

### Configuration Key Reference

| Key | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| **worker_id** | `string` | Yes | Unique hostname or identifier for this worker instance. |
| **credential_id** | `string` | Yes | The ID of the Worker Credential record created in Postgres by the administrator. |
| **private_key_ed25519_env** | `string` | Yes | The name of the environment variable containing the standard base64-encoded Ed25519 private key seed (32 bytes) or full private key (64 bytes). Do not store secrets directly in the JSON file. |
| **heartbeat_interval_ms** | `integer` | No | Frequency in milliseconds at which the worker reports health status to Control (default `5000` / 5s). |
| **health_port** | `integer` | No | Port on which the HTTP `/healthz` and `/readyz` status endpoints are served (default `8090`). |
| **nats.servers** | `array` | Yes | List of NATS server URLs to connect to. |
| **allowed_pools** | `array` | Yes | List of pool mapping objects the worker claims membership in. Each entry must provide `tenant_id` and `pool_id`. Control will only route requests matching these pool IDs to this worker. |
| **capabilities** | `object` | No | Declared worker capabilities validated against Postgres on registration. |
| **capabilities.max_concurrency** | `integer` | No | Maximum number of concurrent request executions allowed on this worker (default `4`). |
| **capabilities.supported_ingress_modes**| `array` | No | List of supported proxy styles. Valid values: `rest`, `mitm`, `connect` (default `["rest"]`). |
| **capabilities.ip_types** | `array` | No | Worker IP categorization, e.g. `["datacenter"]` or `["residential"]`. |
| **upstream_connection_pool** | `object` | No | Optional local HTTP connection pool reuse settings for upstream requests. |
| **upstream_connection_pool.enabled**| `boolean` | No | If `true`, reuse connections to remote servers. If `false`, establish a new connection per request. |

---

## Hardening & Security Best Practices

1. **Ed25519 Secret Isolation**: The worker's private key should be injected strictly via environment variables (e.g. through Kubernetes Secrets or Docker Compose environment injection).
2. **Minimal Credentials**: Workers do not have access to Postgres, Redis, or ClickHouse. They only connect to the NATS cluster. Keep worker NATS credentials restricted so they can only publish and subscribe to the worker-facing subjects (`straw.worker.>`).
3. **No Root Privileges**: Run the worker daemon under a non-privileged system user account.

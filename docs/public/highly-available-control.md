---
sidebar_position: 10
---

# Highly available Control

The optional HA profile lets a readiness-aware load balancer send any request to any Control instance. It adds Redis
for expiring coordination state and enables JetStream for durable runtime configuration. The default `make dev` stack
is unchanged and still requires only NATS.

## Start the adaptable example

Copy the production environment template, replace every secret, and start the standalone profile:

```sh
cp deploy/production/.env.example deploy/production/.env
$EDITOR deploy/production/.env
docker compose --env-file deploy/production/.env \
  -f deploy/production/compose.ha.yml up -d --build
```

HAProxy listens on loopback port `8080` and checks each Control's `/readyz` endpoint on its metrics port. Control
metrics are available on loopback ports `9091` and `9092`. The example is a topology pattern, not a turnkey production
system: replace its single NATS and Redis containers with availability models appropriate to your environment, use
`rediss://` across untrusted networks, terminate public TLS, and move secrets into a secret manager.

## Coordination and ownership

Redis keys use the configured prefix and explicit TTLs:

| State | Ownership and fencing | Expiry |
| --- | --- | --- |
| Worker session, heartbeat, health and capacity | a new registration replaces the session fence; stale heartbeats cannot overwrite it | `worker_ttl_ms`, refreshed by heartbeats |
| Worker failure cooldown | scoped to worker and fenced session | failure window and cooldown durations |
| Sticky route | deployment and sticky-session ID | routing rule TTL, refreshed on use |
| In-flight request | unique Control instance ID | `request_ttl_ms`, renewed while the request context lives |
| Control instance | unique Control instance ID and `active`/`draining` state | `instance_ttl_ms`, refreshed every one third of the TTL |
| Configuration version | monotonic version hint | no TTL; JetStream KV remains authoritative |

Assignment messages and request streams remain idempotent and fenced by request, worker, and worker-session IDs.
Capacity reported by the worker is shared for selection; the worker's assignment acknowledgement remains the final
admission authority when Controls race for the last slot. Administrative cancellation looks up the request owner and
publishes to that instance's NATS cancellation subject. Request payloads never enter Redis.

Runtime configuration requires `runtime_admin.enabled`. Every Control loads the JetStream KV record before serving,
subscribes to fast snapshot invalidation, and also polls the durable record. A missed notification therefore converges
without making Redis a configuration database.

## Failure behavior

- **Control stops:** the load balancer removes it after readiness fails. Other Controls keep using the shared worker
  fleet. Client connections held by the failed process are lost; replay only requests that are safe to replay.
- **Control drains:** SIGTERM fails readiness first, marks the instance `draining`, and waits through the configured
  request deadline plus five seconds for its active requests.
- **Redis is unavailable:** `/healthz` stays `200`, `/readyz` becomes `503`, worker routing fails closed, new sticky
  pins and remote administrative cancellation are unavailable, and durable configuration is untouched. Existing
  request streams can continue until an ownership renewal fails, at which point Control cancels them safely.
- **Redis returns:** the instance lease probe reconnects automatically, readiness returns to `200`, worker heartbeats
  repopulate expired sessions, and routing resumes. No restart is required.
- **NATS is unavailable:** request transport and worker coordination cannot proceed. Use NATS availability and
  monitoring appropriate to the deployment.

## Run the failure drills

The drill script affects only the Compose resources in the HA example. Export the same request token stored in `.env`
before the Control-loss drill:

```sh
export STRAW_AUTH_TOKEN='the-value-from-deploy/production/.env'
deploy/production/failure-drill.sh control-loss
deploy/production/failure-drill.sh redis-outage
```

`control-loss` stops one Control, sends a request through HAProxy, and restarts the instance on exit. `redis-outage`
pauses Redis, waits for the instance lease probes, verifies both readiness endpoints return `503`, and always unpauses
Redis on exit. After either drill, confirm readiness and inspect:

```sh
curl -fsS http://127.0.0.1:9091/readyz
curl -fsS http://127.0.0.1:9091/metrics | grep straw_runtime_state
```

Alert when `straw_runtime_state_available` is `0`, when `straw_runtime_state_errors_total` increases, or when fewer
than two Control readiness targets remain. Also monitor worker heartbeat age and NATS health; Control redundancy does
not replace resilient NATS and Redis deployments.

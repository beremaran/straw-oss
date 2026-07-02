## 29. Operational Behavior

### Control Graceful Shutdown

On shutdown, Control:

1. marks readiness false,
2. stops accepting new client requests,
3. stops initiating new assignments,
4. continues servicing active request-scoped streams,
5. sends cancel for abandoned requests when drain deadline is reached,
6. flushes best-effort telemetry,
7. exits after configured drain timeout.

Defaults:

- readiness removal grace: 5s,
- drain timeout: 60s,
- telemetry flush timeout: 5s.

### Worker Graceful Shutdown

On shutdown, worker:

1. sends heartbeat with `draining=true`,
2. unsubscribes from assignment subject,
3. finishes in-flight requests until their deadlines or worker drain timeout,
4. sends final stopping heartbeat if possible,
5. exits.

### Outage Behavior

| Outage                     | Behavior                                                                                                                       |
|----------------------------|--------------------------------------------------------------------------------------------------------------------------------|
| Postgres unavailable       | Use cached snapshots for existing tenants; config writes fail; new uncached tenants fail                                       |
| Redis unavailable          | Apply configured fail policy for limits/quotas; sticky degrades; worker availability uses short local snapshot then fails safe |
| NATS unavailable           | New request dispatch fails with `transport_unavailable`; in-flight streams fail according to timeout/loss semantics            |
| ClickHouse unavailable     | Request transport continues; telemetry buffers boundedly then drops oldest non-critical events                                 |
| Object storage unavailable | P2 BodyRef requests fail or fall back to direct streaming only if enabled and safe                                             |

Do not claim availability over consistency globally. Use explicit per-subsystem fail policies.

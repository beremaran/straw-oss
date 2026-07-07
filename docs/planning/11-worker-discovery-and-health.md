## 11. Worker Discovery and Health

All Egress implementations — the official worker and custom implementations built on the P2 Egress SDK — use the same
registration and heartbeat protocol. The protocol shape is common; only execution behavior differs.

### Worker State Model

Worker state is modeled as three independent fields:

- `runtime_state`: derived from registration, heartbeat, duplicate-session handling, and cooldown.
- `global_admin_state`: durable platform override (`enabled` or `disabled`).
- `tenant_admin_state`: durable tenant routing override (`enabled` or `disabled`) for that tenant only.

```text
runtime_state: unregistered | registering | registered | ready | degraded | unhealthy | unavailable | dead | draining | cooldown | duplicate_replaced
global_admin_state: enabled | disabled
tenant_admin_state: enabled | disabled
```

Eligibility uses all three fields:

```text
if global_admin_state == disabled: exclude for all tenants
else if tenant_admin_state == disabled: exclude for that tenant
else evaluate runtime_state
```

### Worker Runtime State Machine

```text
unregistered
  -> registering
  -> registered
  -> ready
  -> degraded
  -> unhealthy
  -> unavailable
  -> dead

ready/degraded
  -> draining
  -> dead

ready/degraded/unhealthy/unavailable
  -> cooldown
  -> ready | degraded | unhealthy | unavailable

ready/degraded/unhealthy/unavailable/draining
  -> duplicate_replaced
  -> dead
```

State meanings:

| State                | Routable | Meaning                                                        |
|----------------------|----------|----------------------------------------------------------------|
| `unregistered`       | No       | No active runtime session                                      |
| `registering`        | No       | Registration being validated                                   |
| `registered`         | No       | Registered but not yet heartbeat-ready                         |
| `ready`              | Yes      | Healthy and eligible if capacity/capabilities match            |
| `degraded`           | Optional | Eligible only if tenant/deployment policy permits degraded use |
| `unhealthy`          | No       | Alive but self-reported unhealthy                              |
| `unavailable`        | No       | Heartbeat stale beyond availability timeout                    |
| `dead`               | No       | Removed after dead timeout                                     |
| `draining`           | No new   | Finishes in-flight requests only                               |
| `cooldown`           | No       | Temporary exclusion after repeated failures                    |
| `duplicate_replaced` | No       | Superseded by a newer valid session                            |

Eligibility exclusion precedence is defined in Section 10. Global disable overrides tenant state and runtime health.
Tenant disable overrides runtime health only for that tenant. Draining excludes new assignments even when the worker is
otherwise healthy.

### Registration

Workers register over NATS request/reply on:

```text
straw.v1.control.register
```

Control instances subscribe using the `control` queue group.

Registration includes:

- `worker_id`,
- `executor_type`,
- `credential_id`,
- signed registration token,
- protocol major/minor,
- software version,
- pool IDs,
- tags,
- countries,
- regions,
- IP types,
- supported ingress modes,
- stable egress identity when known,
- max concurrency,
- initial draining state.

Control validates:

- signature,
- credential status,
- tenant scope,
- pool scope,
- capability scope,
- protocol compatibility,
- safe subject-token format for `worker_id`.

A successful registration returns a runtime `session_id`. The worker subscribes to its exact assignment subject only
after receiving `OK`.

### Heartbeats

Workers heartbeat over NATS request/reply on:

```text
straw.v1.control.heartbeat
```

Heartbeats include:

- `worker_id`,
- `session_id`,
- health (`ready`, `degraded`, or `unhealthy`),
- reason,
- active request count,
- max concurrency,
- available capacity,
- optional queue depth,
- draining flag,
- optional worker timestamp for diagnostics.

Control uses receive time, not worker time, for liveness. Heartbeats from stale `session_id`s are ignored for routing
but may be recorded for diagnostics.

### Health Thresholds

Defaults:

| Setting                 |          Default | Meaning                              | Config key                                                          |
|-------------------------|-----------------:|--------------------------------------|---------------------------------------------------------------------|
| Heartbeat interval      |               5s | Worker send cadence                  | `egress.heartbeat.interval_ms`                                      |
| Availability timeout    |              15s | Excluded from new assignments        | `control.worker.availability_timeout_ms`                            |
| Dead timeout            |              30s | Runtime session removed              | `control.worker.dead_timeout_ms`                                    |
| Duplicate-session grace |              10s | Old session drains after replacement | `control.worker.duplicate_session_grace_ms`                         |
| Assignment ack timeout  |               2s | Wait for `AssignAck`                 | `control.worker.assignment_ack_timeout_ms`                          |
| Cooldown trigger        | 3 failures / 60s | Worker excluded from new work        | `control.worker.cooldown_failure_count`, `control.worker.cooldown_window_ms` |
| Cooldown duration       |              30s | Exclusion period                     | `control.worker.cooldown_duration_ms`                               |

### Duplicate Sessions

Only one active session per `worker_id` is routable. New valid registration creates a new `session_id` and replaces the
old session after grace. The old session receives no new assignments during grace. In-flight requests on the old session
may finish until their request deadline or duplicate-session grace deadline, whichever policy chooses; P0 uses the
request deadline unless the worker disconnects.

### Draining and Disable

Draining is runtime state. It excludes new assignments but allows in-flight work until deadline.

Global disable is durable platform admin state. It survives restart and excludes the worker from routing for every tenant
while still allowing registration/heartbeat for observability.

Tenant disable is durable tenant admin state. It survives restart and excludes the worker from routing only for that
tenant. Tenant drain is runtime/ephemeral and excludes new assignments only for that tenant until cleared or until the
worker session ends.

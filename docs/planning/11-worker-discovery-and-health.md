## 11. Worker Discovery and Health

Egress Workers and Provider Adapters use the same registration and heartbeat protocol. Provider Adapters are P2, but the
protocol shape is common.

### Worker Session State Machine

Runtime worker session state is derived from registration, heartbeat, admin state, duplicate-session handling, and
cooldown. P0 uses this state machine:

```text
unregistered
  -> registering
  -> registered
  -> ready
  -> degraded
  -> unavailable
  -> dead

ready/degraded
  -> draining
  -> stopped | dead

ready/degraded/unavailable
  -> cooldown
  -> ready | degraded | unavailable

ready/degraded/unavailable/draining
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
| `unavailable`        | No       | Heartbeat stale beyond availability timeout                    |
| `dead`               | No       | Removed after dead timeout                                     |
| `draining`           | No new   | Finishes in-flight requests only                               |
| `disabled`           | No       | Durable admin exclusion                                        |
| `cooldown`           | No       | Temporary exclusion after repeated failures                    |
| `duplicate_replaced` | No       | Superseded by a newer valid session                            |

Eligibility exclusion precedence is defined in Section 10. Disable is durable admin state and overrides all runtime
health. Draining excludes new assignments even when the worker is otherwise healthy.

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
- health,
- reason,
- active request count,
- max concurrency,
- available capacity,
- optional queue depth,
- draining flag,
- optional worker timestamp for diagnostics.

Control uses receive time, not worker time, for liveness. Heartbeats from stale `session_id`s are ignored for routing
but
may be recorded for diagnostics.

### Health Thresholds

Defaults:

| Setting                 |          Default | Meaning                              | Config key                                                                   |
|-------------------------|-----------------:|--------------------------------------|------------------------------------------------------------------------------|
| Heartbeat interval      |               5s | Worker send cadence                  | `worker.heartbeat_interval_ms`                                               |
| Availability timeout    |              15s | Excluded from new assignments        | `control.worker_availability_timeout_ms`                                     |
| Dead timeout            |              30s | Runtime session removed              | `control.worker_dead_timeout_ms`                                             |
| Duplicate-session grace |              10s | Old session drains after replacement | `control.worker_duplicate_session_grace_ms`                                  |
| Assignment ack timeout  |               2s | Wait for `AssignAck`                 | `control.assignment_ack_timeout_ms`                                          |
| Cooldown trigger        | 3 failures / 60s | Worker excluded from new work        | `control.worker_cooldown_failure_count`, `control.worker_cooldown_window_ms` |
| Cooldown duration       |              30s | Exclusion period                     | `control.worker_cooldown_duration_ms`                                        |

### Duplicate Sessions

Only one active session per `worker_id` is routable. New valid registration creates a new `session_id` and replaces the
old session after grace. The old session receives no new assignments during grace. In-flight requests on the old session
may finish until their request deadline or duplicate-session grace deadline, whichever policy chooses; P0 uses the
request deadline unless the worker disconnects.

### Draining and Disable

Draining is runtime state. It excludes new assignments but allows in-flight work until deadline.

Disable is durable admin state. It survives restart and excludes the worker from routing while still allowing
registration/heartbeat for observability.

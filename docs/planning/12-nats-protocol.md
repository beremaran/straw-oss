## 12. NATS Protocol

### Transport Decision

P0/P1 use Core NATS only. There is no JetStream, no durable message retention, and no redelivery guarantee. All request
execution is synchronous/transient from Straw's perspective.

### Envelope

All NATS messages are binary protobuf `Envelope` messages.

Envelope fields:

- `request_id`,
- `tenant_id`,
- `trace_id`,
- optional serialized trace context,
- `deadline_unix_ms`,
- `protocol_major`,
- `protocol_minor`,
- `attempt`,
- `oneof payload`.

JSON is not used inside NATS.

### Canonical Subjects

| Subject                                                  | Direction                | Payload            | Queue group | Purpose                                                         |
|----------------------------------------------------------|--------------------------|--------------------|-------------|-----------------------------------------------------------------|
| `straw.v1.control.register`                              | Executor → Control       | `RegisterRequest`  | `control`   | Registration request/reply                                      |
| `straw.v1.control.heartbeat`                             | Executor → Control       | `HeartbeatRequest` | `control`   | Heartbeat request/reply                                         |
| `straw.v1.executor.<worker_id>.<session_id>.assign`      | Control → exact executor | `AssignRequest`    | none        | Exact-session assignment request/reply                          |
| `straw.v1.req.<request_id>.<worker_id>.<session_id>.c2e` | Control → executor       | `StreamFrame`      | none        | Request body, tunnel upload, request control, response credit   |
| `straw.v1.req.<request_id>.<worker_id>.<session_id>.e2c` | Executor → Control       | `StreamFrame`      | none        | Response body, tunnel download, executor control, upload credit |

Dot-free safe tokens are required for `request_id`, `worker_id`, and `session_id`. Invalid tokens are rejected. Tenant
IDs are never placed in NATS subjects.

### Direction Rules

`c2e` carries:

- `RequestStart`,
- upload/tunnel `DataFrame`,
- `BodyRef` references for request body,
- `CancelFrame`,
- credit for executor-to-Control response/download bytes.

`e2c` carries:

- `OutboundStartFrame`,
- `ResponseStart`,
- response/tunnel `DataFrame`,
- `TrailersFrame`,
- `EndFrame`,
- `ErrorFrame`,
- `CancelledFrame`,
- credit for Control-to-executor upload bytes.

### Assignment Flow

1. Control sends `AssignRequest` to exact assignment subject.
2. Executor immediately reserves capacity or rejects.
3. Executor replies with `AssignAck`.
4. If accepted, both sides subscribe to request-scoped subjects.
5. Control sends `RequestStart`.
6. Streams proceed under sequence validation and credit-based flow control.

There are no generic NATS retries for assignment. Duplicate assignment is worse than a clean failed attempt.

### Stream Ordering and Sequencing

Each request has two independent ordered stream directions:

- `c2e` for Control-to-executor frames,
- `e2c` for executor-to-Control frames.

Every `StreamFrame` carries:

- `stream_seq`: monotonically increasing unsigned integer, starting at 1 per direction and attempt,
- `attempt`: copied from the Envelope and validated against the active attempt,
- `terminal`: implicit from terminal payload type.

Rules:

- receiver accepts exactly the next expected `stream_seq`,
- duplicate frames with sequence lower than expected are ignored and counted,
- gaps or sequence higher than expected are protocol errors,
- frames after terminal are ignored and counted,
- repeated duplicates, late frames, or invalid sequence behavior contribute to worker cooldown,
- sequence numbers are not reused across attempts.

`DataFrame` additionally carries `offset` for diagnostics and optional integrity checks. `offset` must match the
cumulative
number of data bytes previously accepted in that direction for the same logical byte stream.

### Backpressure

P0 uses byte-credit flow control.

Defaults:

| Setting                      | Default | Config key                                |
|------------------------------|--------:|-------------------------------------------|
| Max frame data bytes         |   1 MiB | `transport.max_frame_data_bytes`          |
| Initial upload credit        |   8 MiB | `transport.initial_upload_credit_bytes`   |
| Initial download credit      |   8 MiB | `transport.initial_download_credit_bytes` |
| Max in-flight upload bytes   |  16 MiB | `transport.max_inflight_upload_bytes`     |
| Max in-flight download bytes |  16 MiB | `transport.max_inflight_download_bytes`   |
| Frame idle timeout           |     15s | `transport.frame_idle_timeout_ms`         |

Credit applies to raw bytes carried in `DataFrame.data`. If bytes are compressed by the client/upstream, credit counts
compressed bytes.

When credit reaches zero, senders stop reading from their upstream source where possible. Control must stop or slow
client reads to avoid unbounded buffering.

### NATS Max Payload Validation

`transport.max_frame_data_bytes` must be less than the effective NATS maximum payload minus protobuf Envelope overhead.
Startup validation must fail if the configured maximum frame data size can produce an Envelope larger than the
configured
or discovered NATS max payload.

P0 recommended rule:

```text
transport.max_frame_data_bytes <= nats.max_payload_bytes - 65536
```

The 64 KiB overhead budget is intentionally conservative and may be replaced by measured encoded-size validation.

### NATS Subject ACLs

NATS credentials are service-level but must still be scoped.

| Principal           | Publish allowed                                                                               | Subscribe allowed                                                                   |
|---------------------|-----------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------|
| Control             | `straw.v1.executor.*.*.assign`, `straw.v1.req.*.*.*.c2e`                                      | `straw.v1.control.register`, `straw.v1.control.heartbeat`, `straw.v1.req.*.*.*.e2c` |
| Worker `worker_id`  | `straw.v1.control.register`, `straw.v1.control.heartbeat`, `straw.v1.req.*.<worker_id>.*.e2c` | `straw.v1.executor.<worker_id>.*.assign`, `straw.v1.req.*.<worker_id>.*.c2e`        |
| Adapter `worker_id` | same as worker                                                                                | same as worker                                                                      |

Tenant authorization remains in Control and worker credential validation. NATS subject credentials prevent broad
cross-subject misuse but are not the tenant authorization source of truth.

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

### Envelope Validation by Payload Type

Different payload types have different mandatory field requirements:

```text
RegisterRequest:
  request_id: empty or generated control-message id
  tenant_id: empty unless credential is single-tenant
  deadline_unix_ms: registration deadline (epoch ms)
  attempt: 0

HeartbeatRequest:
  request_id: empty or generated control-message id
  tenant_id: empty unless credential is single-tenant
  deadline_unix_ms: empty
  attempt: 0

AssignRequest / StreamFrame:
  request_id: required
  tenant_id: required
  deadline_unix_ms: required
  attempt: >= 1
```

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

### Assignment Flow (Subscription Ordering)

To prevent lost messages in Core NATS (which does not retain messages for later subscribers), the assignment flow
enforces strict subscription ordering:

1. Control subscribes to the request-scoped `e2c` subject and **flushes** the subscription.
2. Control sends `AssignRequest` to the exact assignment subject.
3. Executor validates the assignment and reserves capacity.
4. Executor subscribes to request-scoped `c2e` and **flushes** the subscription.
5. Executor replies with `AssignAck`.
6. If `AssignAck` is `ACCEPTED`, Control sends `RequestStart` over the request-scoped `c2e` subject.

Any NATS client subscription used for the request-scoped stream must be flushed or otherwise confirmed before the peer
is allowed to publish to it.

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
cumulative number of data bytes previously accepted in that direction for the same logical byte stream.

### Backpressure and Credit Semantics

P0 uses byte-credit flow control.

Defaults:

| Setting                      | Default | Config key                                     |
|------------------------------|--------:|------------------------------------------------|
| Max frame data bytes         |   1 MiB | `control.transport.max_frame_data_bytes`       |
| Initial upload credit        |   8 MiB | `control.transport.initial_upload_credit_bytes`|
| Initial download credit      |   8 MiB | `control.transport.initial_download_credit_bytes`|
| Max in-flight upload bytes   |  16 MiB | `control.transport.max_inflight_upload_bytes`  |
| Max in-flight download bytes |  16 MiB | `control.transport.max_inflight_download_bytes`|
| Frame idle timeout           |     15s | `control.transport.frame_idle_timeout_ms`      |

Credit rules:

- Initial upload/download credit is implicit from config and included in `AssignRequest` or `RequestStart`.
- `CreditFrame` is sequenced like other stream frames.
- `CreditFrame` grants additional byte credit for `DataFrame` payload bytes only.
- Control frames (`RequestStart`, `CancelFrame`, `ErrorFrame`, `EndFrame`, `TrailersFrame`, `CancelledFrame`) and
  terminal frames **do not consume** byte credit.
- `DataFrame` bytes consume credit after acceptance.
- A receiver should replenish credit after it has processed or released buffered bytes.
- When credit reaches zero, senders stop reading from their upstream source where possible. Control must stop or slow
  client reads to avoid unbounded buffering.

Credit applies to raw bytes carried in `DataFrame.data`. If bytes are compressed by the client/upstream, credit counts
compressed bytes.

When credit reaches zero, senders stop reading from their upstream source where possible. Control must stop or slow
client reads to avoid unbounded buffering.

### NATS Max Payload Validation

`control.transport.max_frame_data_bytes` must be less than the effective NATS maximum payload minus protobuf Envelope
overhead. Startup validation must fail if the configured maximum frame data size can produce an Envelope larger than the
configured or discovered NATS max payload.

P0 recommended rule:

```text
control.transport.max_frame_data_bytes <= nats.max_payload_bytes - 65536
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

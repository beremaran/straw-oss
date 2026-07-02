## 13. Protobuf Contract

### Package

```text
package straw.v1;
Go package: strawpb
Source: proto/straw/v1/straw.proto
```

### Compatibility

- proto3 syntax,
- no protobuf `required` fields,
- mandatory business fields validated after decode,
- unknown fields tolerated,
- unknown enum values rejected for control-plane decisions,
- removed fields reserve numbers and names,
- Buf lint and breaking checks required in CI.

### Core Messages

```protobuf
message Envelope {
  string request_id = 1;
  string tenant_id = 2;
  string trace_id = 3;
  int64 deadline_unix_ms = 4;
  uint32 protocol_major = 5;
  uint32 protocol_minor = 6;
  uint32 attempt = 7;
  bytes trace_context = 8;

  oneof payload {
    RegisterRequest register_request = 20;
    RegisterAck register_ack = 21;
    HeartbeatRequest heartbeat_request = 22;
    HeartbeatAck heartbeat_ack = 23;
    AssignRequest assign_request = 24;
    AssignAck assign_ack = 25;
    StreamFrame stream_frame = 26;
  }
}
```

### Assignment Messages

`AssignRequest` reserves capacity only. It does not carry the HTTP request.

Fields:

- mode: decoded HTTP or raw tunnel,
- deadline,
- expected upload size if known,
- selected route ID,
- selected pool ID,
- selected executor ID,
- stable egress identity if known,
- replayability flag,
- attempt number.

`AssignAck` values:

- `ACCEPTED`,
- `REJECTED_CAPACITY`,
- `REJECTED_DRAINING`,
- `REJECTED_UNSUPPORTED`,
- `REJECTED_AUTH_SCOPE`,
- `REJECTED_ERROR`.

### StreamFrame Payloads

`StreamFrame` has common sequencing fields and a payload `oneof`:

```protobuf
message StreamFrame {
  uint64 stream_seq = 1;
  uint64 attempt = 2;

  oneof payload {
    RequestStart request_start = 10;
    OutboundStartFrame outbound_start = 11;
    ResponseStart response_start = 12;
    DataFrame data = 13;
    CreditFrame credit = 14;
    BodyRefFrame body_ref = 15;
    CancelFrame cancel = 16;
    ErrorFrame error = 17;
    TrailersFrame trailers = 18;
    EndFrame end = 19;
    CancelledFrame cancelled = 20;
  }
}
```

### DataFrame

```protobuf
message DataFrame {
  uint64 offset = 1;
  bytes data = 2;
}
```

`stream_seq` orders frames. `offset` orders byte payloads within a logical byte stream and must equal the cumulative
accepted byte count for that direction and attempt.

### Headers

HTTP headers are ordered repeated pairs:

```protobuf
message Header {
  string name = 1;
  bytes value = 2;
}
```

Maps are not used for headers because order and duplicates matter.

### RequestStart

`RequestStart` carries:

- mode,
- HTTP method,
- absolute URL,
- outbound headers after Straw header stripping,
- routing metadata,
- selected route/pool/executor metadata,
- deadline,
- replayable flag,
- payload capture decision,
- resolved fingerprint instruction,
- ordered injection operations,
- redirect-following policy,
- resolved destination policy bundle,
- policy/version IDs for audit correlation.

Executors never query config and never receive raw admin policy objects. Control sends only resolved request
instructions plus policy/version IDs for audit correlation.

### DestinationPolicy

`DestinationPolicy` is included in `RequestStart` and contains the minimum policy required for Egress-side enforcement:

```protobuf
message DestinationPolicy {
  bool allow_private_ranges = 1;
  bool allow_loopback = 2;
  bool allow_link_local = 3;
  bool allow_metadata_ips = 4;
  repeated string denied_cidrs = 5;
  repeated string allowed_cidrs = 6;
  repeated string denied_host_suffixes = 7;
  repeated string denied_cname_suffixes = 8;
  SniHostMismatchPolicy sni_host_mismatch_policy = 9;
  RedirectPolicy redirect_policy = 10;
  string policy_version = 11;
}
```

P0 default policy denies private, loopback, link-local, multicast, and cloud metadata ranges unless a tenant admin
explicitly
allows them for the tenant or deployment.

### OutboundStartFrame

`OutboundStartFrame` is emitted by Egress before DNS/connect or before delegating to an upstream proxy. It carries:

- target host,
- target port,
- selected upstream proxy ID if any,
- attempt,
- timestamp from worker for diagnostics only.

Control does not use worker timestamps for deadlines or liveness.

### BodyRef

P2 adds BodyRef variants:

- `S3BodyRef`,
- `DirectStreamRef`.

P0 supports only inline NATS `DataFrame` bodies derived from REST `inline_base64` input.

### Error Facts and ErrorResponse

Internal protobuf `Error` is not identical to public REST `ErrorResponse`.

Executors emit constrained failure facts. Control maps them to public error codes.

`Error` fields:

- internal failure fact,
- retryable hint,
- operator message,
- details map,
- optional upstream status,
- optional timeout type.

Public `ErrorResponse` fields:

```protobuf
message ErrorResponse {
  ErrorCategory category = 1;
  ErrorCode code = 2;
  string message = 3;
  bool retryable = 4;
  uint64 retry_after_ms = 5;
  string request_id = 6;
  optional uint32 upstream_status = 7;
  optional TimeoutType timeout_type = 8;
  map<string, string> details = 9;
}
```

`worker_id` and `session_id` are never exposed to clients. `details` may contain bounded public-safe diagnostics such as
`direction=request|response`, `limit_bytes`, or `retry_policy`, but never internal topology identifiers or secrets.

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
syntax = "proto3";
package straw.v1;

enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  ERROR_CODE_AUTH_FAILURE = 1;
  ERROR_CODE_TENANT_NOT_FOUND = 2;
  ERROR_CODE_INSUFFICIENT_PERMISSIONS = 3;
  ERROR_CODE_RATE_LIMIT_EXCEEDED = 4;
  ERROR_CODE_QUOTA_EXHAUSTED = 5;
  ERROR_CODE_INVALID_REQUEST = 6;
  ERROR_CODE_DESTINATION_DENIED = 7;
  ERROR_CODE_HEADER_INJECTION_FAILED = 8;
  ERROR_CODE_CONFLICT = 9;
  ERROR_CODE_UNSUPPORTED_INGRESS_MODE = 10;
  ERROR_CODE_ROUTE_NO_MATCH = 100;
  ERROR_CODE_ROUTE_UNAVAILABLE = 101;
  ERROR_CODE_STICKY_SESSION_UNAVAILABLE = 102;
  ERROR_CODE_EXECUTOR_CAPACITY_EXHAUSTED = 103;
  ERROR_CODE_ASSIGNMENT_TIMEOUT = 200;
  ERROR_CODE_WORKER_DISCONNECTED = 201;
  ERROR_CODE_TRANSPORT_UNAVAILABLE = 202;
  ERROR_CODE_PROTOCOL_ERROR = 203;
  ERROR_CODE_TIMEOUT_EXCEEDED = 204;
  ERROR_CODE_UNSUPPORTED_FINGERPRINT = 205;
  ERROR_CODE_UPSTREAM_DNS_FAILURE = 300;
  ERROR_CODE_UPSTREAM_TLS_FAILURE = 301;
  ERROR_CODE_UPSTREAM_CONNECTION_REFUSED = 302;
  ERROR_CODE_UPSTREAM_CONNECT_TIMEOUT = 303;
  ERROR_CODE_UPSTREAM_RESET = 304;
  ERROR_CODE_UPSTREAM_PROXY_FAILURE = 305;
  ERROR_CODE_STREAM_UPLOAD_ABORTED = 400;
  ERROR_CODE_STREAM_DOWNLOAD_ABORTED = 401;
  ERROR_CODE_BODY_REF_UNAVAILABLE = 402;
  ERROR_CODE_BODY_TOO_LARGE = 403;
  ERROR_CODE_CONTROL_INTERNAL_ERROR = 500;
  ERROR_CODE_EXECUTOR_INTERNAL_ERROR = 501;
  ERROR_CODE_CANCELLED = 502;
}

enum ErrorCategory {
  ERROR_CATEGORY_UNSPECIFIED = 0;
  ERROR_CATEGORY_CLIENT = 1;
  ERROR_CATEGORY_ROUTING = 2;
  ERROR_CATEGORY_TRANSPORT = 3;
  ERROR_CATEGORY_EGRESS = 4;
  ERROR_CATEGORY_STREAMING = 5;
  ERROR_CATEGORY_CONTROL = 6;
}

enum TimeoutType {
  TIMEOUT_TYPE_UNSPECIFIED = 0;
  TIMEOUT_TYPE_ASSIGNMENT_TIMEOUT = 1;
  TIMEOUT_TYPE_CONNECT_TIMEOUT = 2;
  TIMEOUT_TYPE_RESPONSE_HEADER_TIMEOUT = 3;
  TIMEOUT_TYPE_IDLE_TIMEOUT = 4;
  TIMEOUT_TYPE_UPLOAD_TIMEOUT = 5;
  TIMEOUT_TYPE_DOWNLOAD_TIMEOUT = 6;
  TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT = 7;
}

enum AssignAckCode {
  ASSIGN_ACK_CODE_UNSPECIFIED = 0;
  ASSIGN_ACK_ACCEPTED = 1;
  ASSIGN_ACK_REJECTED_CAPACITY = 2;
  ASSIGN_ACK_REJECTED_DRAINING = 3;
  ASSIGN_ACK_REJECTED_UNSUPPORTED = 4;
  ASSIGN_ACK_REJECTED_AUTH_SCOPE = 5;
  ASSIGN_ACK_REJECTED_ERROR = 6;
}

enum RequestMode {
  REQUEST_MODE_UNSPECIFIED = 0;
  REQUEST_MODE_DECODED_HTTP = 1;
  REQUEST_MODE_RAW_TUNNEL = 2;
}

enum WorkerHealth {
  WORKER_HEALTH_UNSPECIFIED = 0;
  WORKER_HEALTH_READY = 1;
  WORKER_HEALTH_DEGRADED = 2;
  WORKER_HEALTH_UNHEALTHY = 3;
}

enum SniHostMismatchPolicy {
  SNI_HOST_MISMATCH_STRICT = 0;
  SNI_HOST_MISMATCH_WARN = 1;
  SNI_HOST_MISMATCH_ALLOW = 2;
}

enum RedirectPolicy {
  REDIRECT_POLICY_NO_FOLLOW = 0;
  REDIRECT_POLICY_FOLLOW_STRICT = 1;
}

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

message RegisterRequest {
  string worker_id = 1;
  string executor_type = 2;
  string credential_id = 3;
  bytes signed_token = 4;
  uint32 protocol_major = 5;
  uint32 protocol_minor = 6;
  string software_version = 7;
  message PoolRef {
    string tenant_id = 1;
    string pool_id = 2;
  }
  repeated PoolRef allowed_pools = 8;
  repeated string tags = 9;
  repeated string countries = 10;
  repeated string regions = 11;
  repeated string ip_types = 12;
  repeated string supported_ingress_modes = 13;
  string stable_egress_identity = 14;
  uint32 max_concurrency = 15;
  bool initial_draining = 16;
}

message RegisterAck {
  bool ok = 1;
  string session_id = 2;
  string error = 3;
}

message HeartbeatRequest {
  string worker_id = 1;
  string session_id = 2;
  WorkerHealth health = 3;
  string reason = 4;
  uint32 active_requests = 5;
  uint32 max_concurrency = 6;
  uint32 available_capacity = 7;
  optional uint32 queue_depth = 8;
  bool draining = 9;
  int64 worker_timestamp_ms = 10;
}

message HeartbeatAck {
  bool ok = 1;
  string error = 2;
}

message AssignRequest {
  RequestMode mode = 1;
  int64 deadline_unix_ms = 2;
  int64 expected_upload_bytes = 3;
  string selected_route_id = 4;
  string selected_pool_id = 5;
  string selected_executor_id = 6;
  string stable_egress_identity = 7;
  bool replayable = 8;
  uint32 attempt = 9;
  string policy_version = 10;
  uint64 initial_upload_credit_bytes = 11;
  uint64 initial_download_credit_bytes = 12;
  uint64 max_inflight_upload_bytes = 13;
  uint64 max_inflight_download_bytes = 14;
}

message AssignAck {
  AssignAckCode code = 1;
  string error = 2;
}

message StreamFrame {
  uint64 stream_seq = 1;
  uint32 attempt = 2;

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

message DataFrame {
  uint64 offset = 1;
  bytes data = 2;
}
```

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
  bool allow_multicast = 4;
  bool allow_metadata_ips = 5;
  repeated string denied_cidrs = 6;
  repeated string allowed_cidrs = 7;
  repeated string denied_host_suffixes = 8;
  repeated string denied_cname_suffixes = 9;
  SniHostMismatchPolicy sni_host_mismatch_policy = 10;
  RedirectPolicy redirect_policy = 11;
  string policy_version = 12;
  DestinationResolutionMode resolution_mode = 13;
}

enum DestinationResolutionMode {
  DESTINATION_RESOLUTION_MODE_UNSPECIFIED = 0;
  DESTINATION_RESOLUTION_DIRECT_LOCAL = 1;
  DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE = 2;
  DESTINATION_RESOLUTION_PROVIDER_ADAPTER = 3;
}
```

P0 default policy denies private, loopback, link-local, multicast, and cloud metadata ranges unless a tenant admin
explicitly allows them for the tenant or deployment.

`DESTINATION_RESOLUTION_DIRECT_LOCAL`: Worker resolves, validates, and connects to the validated IP.
`DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE`: Worker cannot prove resolved-IP policy. Allowed only if the upstream
proxy is trusted to enforce equivalent policy, or if tenant/deployment policy explicitly accepts remote-resolution risk.
`DESTINATION_RESOLUTION_PROVIDER_ADAPTER`: Adapter must enforce equivalent destination policy and report constrained
facts back to Control.

### OutboundStartFrame

`OutboundStartFrame` is emitted by Egress before DNS/connect or before delegating to an upstream proxy. It carries:

- target host,
- target port,
- selected upstream proxy ID if any,
- attempt,
- timestamp from worker for diagnostics only.

Control does not use worker timestamps for deadlines or liveness.

### BodyRef

The wire contract defines BodyRef in P0 so generated protobuf code is complete. P0 rejects any `BodyRefFrame` during
REST validation and stream validation. P2 enables the variants after the related transport decision is made.

```protobuf
message BodyRefFrame {
  oneof ref {
    S3BodyRef s3 = 1;
    DirectStreamRef direct_stream = 2;
  }
  uint64 expected_size_bytes = 10;
  string sha256_hex = 11;
}

message S3BodyRef {
  string object_key = 1;
  string signed_url = 2;
  int64 expires_unix_ms = 3;
}

message DirectStreamRef {
  string endpoint = 1;
  string stream_id = 2;
  int64 expires_unix_ms = 3;
}
```

P2 BodyRef variants:

- `S3BodyRef`,
- `DirectStreamRef`.

P0 supports only inline NATS `DataFrame` bodies derived from REST `inline_base64` input.

### Additional StreamFrame Payloads

```protobuf
message CreditFrame {
  uint64 upload_credit_bytes = 1;
  uint64 download_credit_bytes = 2;
}

message ErrorFrame {
  ErrorCode code = 1;
  ErrorCategory category = 2;
  string message = 3;
  bool retryable = 4;
  uint64 retry_after_ms = 5;
  optional uint32 upstream_status = 6;
  optional TimeoutType timeout_type = 7;
  map<string, string> details = 8;
}

message EndFrame {
  bool success = 1;
}

message CancelFrame {
  string reason = 1;
}

message CancelledFrame {
  string reason = 1;
}

message TrailersFrame {
  repeated Header headers = 1;
}

message RequestStart {
  RequestMode mode = 1;
  string method = 2;
  string url = 3;
  repeated Header headers = 4;
  map<string, string> routing_metadata = 5;
  string selected_route_id = 6;
  string selected_pool_id = 7;
  int64 deadline_unix_ms = 8;
  bool replayable = 9;
  string payload_capture_decision = 10;
  string fingerprint_instruction = 11;
  repeated InjectionOperation injection_operations = 12;
  RedirectPolicy redirect_policy = 13;
  DestinationPolicy destination_policy = 14;
  string policy_version = 15;
}

message InjectionOperation {
  string op = 1;
  string header_name = 2;
  bytes value = 3;
}

message ResponseStart {
  uint32 status = 1;
  repeated Header headers = 2;
}
```

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

### JSON Representation Rules

For public REST ErrorResponse, enum values are represented as **lower-snake-case** strings (e.g., `"auth_failure"`, `"routing"`) that match the canonical error registry.
`retry_after_ms` is omitted when zero. `details` values are always strings. `request_id` is always present, even for
malformed requests where it was generated by Control.

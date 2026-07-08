from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class ErrorCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ERROR_CODE_UNSPECIFIED: _ClassVar[ErrorCode]
    ERROR_CODE_AUTH_FAILURE: _ClassVar[ErrorCode]
    ERROR_CODE_TENANT_NOT_FOUND: _ClassVar[ErrorCode]
    ERROR_CODE_INSUFFICIENT_PERMISSIONS: _ClassVar[ErrorCode]
    ERROR_CODE_RATE_LIMIT_EXCEEDED: _ClassVar[ErrorCode]
    ERROR_CODE_QUOTA_EXHAUSTED: _ClassVar[ErrorCode]
    ERROR_CODE_INVALID_REQUEST: _ClassVar[ErrorCode]
    ERROR_CODE_DESTINATION_DENIED: _ClassVar[ErrorCode]
    ERROR_CODE_HEADER_INJECTION_FAILED: _ClassVar[ErrorCode]
    ERROR_CODE_CONFLICT: _ClassVar[ErrorCode]
    ERROR_CODE_UNSUPPORTED_INGRESS_MODE: _ClassVar[ErrorCode]
    ERROR_CODE_ROUTE_NO_MATCH: _ClassVar[ErrorCode]
    ERROR_CODE_ROUTE_UNAVAILABLE: _ClassVar[ErrorCode]
    ERROR_CODE_STICKY_SESSION_UNAVAILABLE: _ClassVar[ErrorCode]
    ERROR_CODE_EXECUTOR_CAPACITY_EXHAUSTED: _ClassVar[ErrorCode]
    ERROR_CODE_ASSIGNMENT_TIMEOUT: _ClassVar[ErrorCode]
    ERROR_CODE_WORKER_DISCONNECTED: _ClassVar[ErrorCode]
    ERROR_CODE_TRANSPORT_UNAVAILABLE: _ClassVar[ErrorCode]
    ERROR_CODE_PROTOCOL_ERROR: _ClassVar[ErrorCode]
    ERROR_CODE_TIMEOUT_EXCEEDED: _ClassVar[ErrorCode]
    ERROR_CODE_UNSUPPORTED_FINGERPRINT: _ClassVar[ErrorCode]
    ERROR_CODE_UPSTREAM_DNS_FAILURE: _ClassVar[ErrorCode]
    ERROR_CODE_UPSTREAM_TLS_FAILURE: _ClassVar[ErrorCode]
    ERROR_CODE_UPSTREAM_CONNECTION_REFUSED: _ClassVar[ErrorCode]
    ERROR_CODE_UPSTREAM_CONNECT_TIMEOUT: _ClassVar[ErrorCode]
    ERROR_CODE_UPSTREAM_RESET: _ClassVar[ErrorCode]
    ERROR_CODE_UPSTREAM_PROXY_FAILURE: _ClassVar[ErrorCode]
    ERROR_CODE_STREAM_UPLOAD_ABORTED: _ClassVar[ErrorCode]
    ERROR_CODE_STREAM_DOWNLOAD_ABORTED: _ClassVar[ErrorCode]
    ERROR_CODE_BODY_REF_UNAVAILABLE: _ClassVar[ErrorCode]
    ERROR_CODE_BODY_TOO_LARGE: _ClassVar[ErrorCode]
    ERROR_CODE_CONTROL_INTERNAL_ERROR: _ClassVar[ErrorCode]
    ERROR_CODE_EXECUTOR_INTERNAL_ERROR: _ClassVar[ErrorCode]
    ERROR_CODE_CANCELLED: _ClassVar[ErrorCode]

class ErrorCategory(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ERROR_CATEGORY_UNSPECIFIED: _ClassVar[ErrorCategory]
    ERROR_CATEGORY_CLIENT: _ClassVar[ErrorCategory]
    ERROR_CATEGORY_ROUTING: _ClassVar[ErrorCategory]
    ERROR_CATEGORY_TRANSPORT: _ClassVar[ErrorCategory]
    ERROR_CATEGORY_EGRESS: _ClassVar[ErrorCategory]
    ERROR_CATEGORY_STREAMING: _ClassVar[ErrorCategory]
    ERROR_CATEGORY_CONTROL: _ClassVar[ErrorCategory]

class TimeoutType(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    TIMEOUT_TYPE_UNSPECIFIED: _ClassVar[TimeoutType]
    TIMEOUT_TYPE_ASSIGNMENT_TIMEOUT: _ClassVar[TimeoutType]
    TIMEOUT_TYPE_CONNECT_TIMEOUT: _ClassVar[TimeoutType]
    TIMEOUT_TYPE_RESPONSE_HEADER_TIMEOUT: _ClassVar[TimeoutType]
    TIMEOUT_TYPE_IDLE_TIMEOUT: _ClassVar[TimeoutType]
    TIMEOUT_TYPE_UPLOAD_TIMEOUT: _ClassVar[TimeoutType]
    TIMEOUT_TYPE_DOWNLOAD_TIMEOUT: _ClassVar[TimeoutType]
    TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT: _ClassVar[TimeoutType]

class AssignAckCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ASSIGN_ACK_CODE_UNSPECIFIED: _ClassVar[AssignAckCode]
    ASSIGN_ACK_ACCEPTED: _ClassVar[AssignAckCode]
    ASSIGN_ACK_REJECTED_CAPACITY: _ClassVar[AssignAckCode]
    ASSIGN_ACK_REJECTED_DRAINING: _ClassVar[AssignAckCode]
    ASSIGN_ACK_REJECTED_UNSUPPORTED: _ClassVar[AssignAckCode]
    ASSIGN_ACK_REJECTED_AUTH_SCOPE: _ClassVar[AssignAckCode]
    ASSIGN_ACK_REJECTED_ERROR: _ClassVar[AssignAckCode]

class RequestMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    REQUEST_MODE_UNSPECIFIED: _ClassVar[RequestMode]
    REQUEST_MODE_DECODED_HTTP: _ClassVar[RequestMode]
    REQUEST_MODE_RAW_TUNNEL: _ClassVar[RequestMode]

class WorkerHealth(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    WORKER_HEALTH_UNSPECIFIED: _ClassVar[WorkerHealth]
    WORKER_HEALTH_READY: _ClassVar[WorkerHealth]
    WORKER_HEALTH_DEGRADED: _ClassVar[WorkerHealth]
    WORKER_HEALTH_UNHEALTHY: _ClassVar[WorkerHealth]

class SniHostMismatchPolicy(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SNI_HOST_MISMATCH_STRICT: _ClassVar[SniHostMismatchPolicy]
    SNI_HOST_MISMATCH_WARN: _ClassVar[SniHostMismatchPolicy]
    SNI_HOST_MISMATCH_ALLOW: _ClassVar[SniHostMismatchPolicy]

class RedirectPolicy(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    REDIRECT_POLICY_NO_FOLLOW: _ClassVar[RedirectPolicy]
    REDIRECT_POLICY_FOLLOW_STRICT: _ClassVar[RedirectPolicy]

class DestinationResolutionMode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    DESTINATION_RESOLUTION_MODE_UNSPECIFIED: _ClassVar[DestinationResolutionMode]
    DESTINATION_RESOLUTION_DIRECT_LOCAL: _ClassVar[DestinationResolutionMode]
    DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE: _ClassVar[DestinationResolutionMode]
    DESTINATION_RESOLUTION_EXECUTOR_DELEGATED: _ClassVar[DestinationResolutionMode]
ERROR_CODE_UNSPECIFIED: ErrorCode
ERROR_CODE_AUTH_FAILURE: ErrorCode
ERROR_CODE_TENANT_NOT_FOUND: ErrorCode
ERROR_CODE_INSUFFICIENT_PERMISSIONS: ErrorCode
ERROR_CODE_RATE_LIMIT_EXCEEDED: ErrorCode
ERROR_CODE_QUOTA_EXHAUSTED: ErrorCode
ERROR_CODE_INVALID_REQUEST: ErrorCode
ERROR_CODE_DESTINATION_DENIED: ErrorCode
ERROR_CODE_HEADER_INJECTION_FAILED: ErrorCode
ERROR_CODE_CONFLICT: ErrorCode
ERROR_CODE_UNSUPPORTED_INGRESS_MODE: ErrorCode
ERROR_CODE_ROUTE_NO_MATCH: ErrorCode
ERROR_CODE_ROUTE_UNAVAILABLE: ErrorCode
ERROR_CODE_STICKY_SESSION_UNAVAILABLE: ErrorCode
ERROR_CODE_EXECUTOR_CAPACITY_EXHAUSTED: ErrorCode
ERROR_CODE_ASSIGNMENT_TIMEOUT: ErrorCode
ERROR_CODE_WORKER_DISCONNECTED: ErrorCode
ERROR_CODE_TRANSPORT_UNAVAILABLE: ErrorCode
ERROR_CODE_PROTOCOL_ERROR: ErrorCode
ERROR_CODE_TIMEOUT_EXCEEDED: ErrorCode
ERROR_CODE_UNSUPPORTED_FINGERPRINT: ErrorCode
ERROR_CODE_UPSTREAM_DNS_FAILURE: ErrorCode
ERROR_CODE_UPSTREAM_TLS_FAILURE: ErrorCode
ERROR_CODE_UPSTREAM_CONNECTION_REFUSED: ErrorCode
ERROR_CODE_UPSTREAM_CONNECT_TIMEOUT: ErrorCode
ERROR_CODE_UPSTREAM_RESET: ErrorCode
ERROR_CODE_UPSTREAM_PROXY_FAILURE: ErrorCode
ERROR_CODE_STREAM_UPLOAD_ABORTED: ErrorCode
ERROR_CODE_STREAM_DOWNLOAD_ABORTED: ErrorCode
ERROR_CODE_BODY_REF_UNAVAILABLE: ErrorCode
ERROR_CODE_BODY_TOO_LARGE: ErrorCode
ERROR_CODE_CONTROL_INTERNAL_ERROR: ErrorCode
ERROR_CODE_EXECUTOR_INTERNAL_ERROR: ErrorCode
ERROR_CODE_CANCELLED: ErrorCode
ERROR_CATEGORY_UNSPECIFIED: ErrorCategory
ERROR_CATEGORY_CLIENT: ErrorCategory
ERROR_CATEGORY_ROUTING: ErrorCategory
ERROR_CATEGORY_TRANSPORT: ErrorCategory
ERROR_CATEGORY_EGRESS: ErrorCategory
ERROR_CATEGORY_STREAMING: ErrorCategory
ERROR_CATEGORY_CONTROL: ErrorCategory
TIMEOUT_TYPE_UNSPECIFIED: TimeoutType
TIMEOUT_TYPE_ASSIGNMENT_TIMEOUT: TimeoutType
TIMEOUT_TYPE_CONNECT_TIMEOUT: TimeoutType
TIMEOUT_TYPE_RESPONSE_HEADER_TIMEOUT: TimeoutType
TIMEOUT_TYPE_IDLE_TIMEOUT: TimeoutType
TIMEOUT_TYPE_UPLOAD_TIMEOUT: TimeoutType
TIMEOUT_TYPE_DOWNLOAD_TIMEOUT: TimeoutType
TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT: TimeoutType
ASSIGN_ACK_CODE_UNSPECIFIED: AssignAckCode
ASSIGN_ACK_ACCEPTED: AssignAckCode
ASSIGN_ACK_REJECTED_CAPACITY: AssignAckCode
ASSIGN_ACK_REJECTED_DRAINING: AssignAckCode
ASSIGN_ACK_REJECTED_UNSUPPORTED: AssignAckCode
ASSIGN_ACK_REJECTED_AUTH_SCOPE: AssignAckCode
ASSIGN_ACK_REJECTED_ERROR: AssignAckCode
REQUEST_MODE_UNSPECIFIED: RequestMode
REQUEST_MODE_DECODED_HTTP: RequestMode
REQUEST_MODE_RAW_TUNNEL: RequestMode
WORKER_HEALTH_UNSPECIFIED: WorkerHealth
WORKER_HEALTH_READY: WorkerHealth
WORKER_HEALTH_DEGRADED: WorkerHealth
WORKER_HEALTH_UNHEALTHY: WorkerHealth
SNI_HOST_MISMATCH_STRICT: SniHostMismatchPolicy
SNI_HOST_MISMATCH_WARN: SniHostMismatchPolicy
SNI_HOST_MISMATCH_ALLOW: SniHostMismatchPolicy
REDIRECT_POLICY_NO_FOLLOW: RedirectPolicy
REDIRECT_POLICY_FOLLOW_STRICT: RedirectPolicy
DESTINATION_RESOLUTION_MODE_UNSPECIFIED: DestinationResolutionMode
DESTINATION_RESOLUTION_DIRECT_LOCAL: DestinationResolutionMode
DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE: DestinationResolutionMode
DESTINATION_RESOLUTION_EXECUTOR_DELEGATED: DestinationResolutionMode

class Envelope(_message.Message):
    __slots__ = ("request_id", "tenant_id", "trace_id", "deadline_unix_ms", "protocol_major", "protocol_minor", "attempt", "trace_context", "register_request", "register_ack", "heartbeat_request", "heartbeat_ack", "assign_request", "assign_ack", "stream_frame", "log_event")
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    DEADLINE_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_MAJOR_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_MINOR_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_FIELD_NUMBER: _ClassVar[int]
    TRACE_CONTEXT_FIELD_NUMBER: _ClassVar[int]
    REGISTER_REQUEST_FIELD_NUMBER: _ClassVar[int]
    REGISTER_ACK_FIELD_NUMBER: _ClassVar[int]
    HEARTBEAT_REQUEST_FIELD_NUMBER: _ClassVar[int]
    HEARTBEAT_ACK_FIELD_NUMBER: _ClassVar[int]
    ASSIGN_REQUEST_FIELD_NUMBER: _ClassVar[int]
    ASSIGN_ACK_FIELD_NUMBER: _ClassVar[int]
    STREAM_FRAME_FIELD_NUMBER: _ClassVar[int]
    LOG_EVENT_FIELD_NUMBER: _ClassVar[int]
    request_id: str
    tenant_id: str
    trace_id: str
    deadline_unix_ms: int
    protocol_major: int
    protocol_minor: int
    attempt: int
    trace_context: bytes
    register_request: RegisterRequest
    register_ack: RegisterAck
    heartbeat_request: HeartbeatRequest
    heartbeat_ack: HeartbeatAck
    assign_request: AssignRequest
    assign_ack: AssignAck
    stream_frame: StreamFrame
    log_event: LogEvent
    def __init__(self, request_id: _Optional[str] = ..., tenant_id: _Optional[str] = ..., trace_id: _Optional[str] = ..., deadline_unix_ms: _Optional[int] = ..., protocol_major: _Optional[int] = ..., protocol_minor: _Optional[int] = ..., attempt: _Optional[int] = ..., trace_context: _Optional[bytes] = ..., register_request: _Optional[_Union[RegisterRequest, _Mapping]] = ..., register_ack: _Optional[_Union[RegisterAck, _Mapping]] = ..., heartbeat_request: _Optional[_Union[HeartbeatRequest, _Mapping]] = ..., heartbeat_ack: _Optional[_Union[HeartbeatAck, _Mapping]] = ..., assign_request: _Optional[_Union[AssignRequest, _Mapping]] = ..., assign_ack: _Optional[_Union[AssignAck, _Mapping]] = ..., stream_frame: _Optional[_Union[StreamFrame, _Mapping]] = ..., log_event: _Optional[_Union[LogEvent, _Mapping]] = ...) -> None: ...

class LogEvent(_message.Message):
    __slots__ = ("timestamp_unix_ms", "service", "level", "message", "request_id", "tenant_id", "trace_id", "worker_id", "error_code", "extra")
    class ExtraEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    TIMESTAMP_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    SERVICE_FIELD_NUMBER: _ClassVar[int]
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    WORKER_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    EXTRA_FIELD_NUMBER: _ClassVar[int]
    timestamp_unix_ms: int
    service: str
    level: str
    message: str
    request_id: str
    tenant_id: str
    trace_id: str
    worker_id: str
    error_code: str
    extra: _containers.ScalarMap[str, str]
    def __init__(self, timestamp_unix_ms: _Optional[int] = ..., service: _Optional[str] = ..., level: _Optional[str] = ..., message: _Optional[str] = ..., request_id: _Optional[str] = ..., tenant_id: _Optional[str] = ..., trace_id: _Optional[str] = ..., worker_id: _Optional[str] = ..., error_code: _Optional[str] = ..., extra: _Optional[_Mapping[str, str]] = ...) -> None: ...

class RegisterRequest(_message.Message):
    __slots__ = ("worker_id", "executor_type", "credential_id", "signed_token", "protocol_major", "protocol_minor", "software_version", "allowed_pools", "tags", "countries", "regions", "ip_types", "supported_ingress_modes", "stable_egress_identity", "max_concurrency", "initial_draining", "nonce", "issued_at_unix_ms")
    class PoolRef(_message.Message):
        __slots__ = ("tenant_id", "pool_id")
        TENANT_ID_FIELD_NUMBER: _ClassVar[int]
        POOL_ID_FIELD_NUMBER: _ClassVar[int]
        tenant_id: str
        pool_id: str
        def __init__(self, tenant_id: _Optional[str] = ..., pool_id: _Optional[str] = ...) -> None: ...
    WORKER_ID_FIELD_NUMBER: _ClassVar[int]
    EXECUTOR_TYPE_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_ID_FIELD_NUMBER: _ClassVar[int]
    SIGNED_TOKEN_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_MAJOR_FIELD_NUMBER: _ClassVar[int]
    PROTOCOL_MINOR_FIELD_NUMBER: _ClassVar[int]
    SOFTWARE_VERSION_FIELD_NUMBER: _ClassVar[int]
    ALLOWED_POOLS_FIELD_NUMBER: _ClassVar[int]
    TAGS_FIELD_NUMBER: _ClassVar[int]
    COUNTRIES_FIELD_NUMBER: _ClassVar[int]
    REGIONS_FIELD_NUMBER: _ClassVar[int]
    IP_TYPES_FIELD_NUMBER: _ClassVar[int]
    SUPPORTED_INGRESS_MODES_FIELD_NUMBER: _ClassVar[int]
    STABLE_EGRESS_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    MAX_CONCURRENCY_FIELD_NUMBER: _ClassVar[int]
    INITIAL_DRAINING_FIELD_NUMBER: _ClassVar[int]
    NONCE_FIELD_NUMBER: _ClassVar[int]
    ISSUED_AT_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    worker_id: str
    executor_type: str
    credential_id: str
    signed_token: bytes
    protocol_major: int
    protocol_minor: int
    software_version: str
    allowed_pools: _containers.RepeatedCompositeFieldContainer[RegisterRequest.PoolRef]
    tags: _containers.RepeatedScalarFieldContainer[str]
    countries: _containers.RepeatedScalarFieldContainer[str]
    regions: _containers.RepeatedScalarFieldContainer[str]
    ip_types: _containers.RepeatedScalarFieldContainer[str]
    supported_ingress_modes: _containers.RepeatedScalarFieldContainer[str]
    stable_egress_identity: str
    max_concurrency: int
    initial_draining: bool
    nonce: bytes
    issued_at_unix_ms: int
    def __init__(self, worker_id: _Optional[str] = ..., executor_type: _Optional[str] = ..., credential_id: _Optional[str] = ..., signed_token: _Optional[bytes] = ..., protocol_major: _Optional[int] = ..., protocol_minor: _Optional[int] = ..., software_version: _Optional[str] = ..., allowed_pools: _Optional[_Iterable[_Union[RegisterRequest.PoolRef, _Mapping]]] = ..., tags: _Optional[_Iterable[str]] = ..., countries: _Optional[_Iterable[str]] = ..., regions: _Optional[_Iterable[str]] = ..., ip_types: _Optional[_Iterable[str]] = ..., supported_ingress_modes: _Optional[_Iterable[str]] = ..., stable_egress_identity: _Optional[str] = ..., max_concurrency: _Optional[int] = ..., initial_draining: _Optional[bool] = ..., nonce: _Optional[bytes] = ..., issued_at_unix_ms: _Optional[int] = ...) -> None: ...

class RegisterAck(_message.Message):
    __slots__ = ("ok", "session_id", "error")
    OK_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    ok: bool
    session_id: str
    error: str
    def __init__(self, ok: _Optional[bool] = ..., session_id: _Optional[str] = ..., error: _Optional[str] = ...) -> None: ...

class HeartbeatRequest(_message.Message):
    __slots__ = ("worker_id", "session_id", "health", "reason", "active_requests", "max_concurrency", "available_capacity", "queue_depth", "draining", "worker_timestamp_ms")
    WORKER_ID_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    HEALTH_FIELD_NUMBER: _ClassVar[int]
    REASON_FIELD_NUMBER: _ClassVar[int]
    ACTIVE_REQUESTS_FIELD_NUMBER: _ClassVar[int]
    MAX_CONCURRENCY_FIELD_NUMBER: _ClassVar[int]
    AVAILABLE_CAPACITY_FIELD_NUMBER: _ClassVar[int]
    QUEUE_DEPTH_FIELD_NUMBER: _ClassVar[int]
    DRAINING_FIELD_NUMBER: _ClassVar[int]
    WORKER_TIMESTAMP_MS_FIELD_NUMBER: _ClassVar[int]
    worker_id: str
    session_id: str
    health: WorkerHealth
    reason: str
    active_requests: int
    max_concurrency: int
    available_capacity: int
    queue_depth: int
    draining: bool
    worker_timestamp_ms: int
    def __init__(self, worker_id: _Optional[str] = ..., session_id: _Optional[str] = ..., health: _Optional[_Union[WorkerHealth, str]] = ..., reason: _Optional[str] = ..., active_requests: _Optional[int] = ..., max_concurrency: _Optional[int] = ..., available_capacity: _Optional[int] = ..., queue_depth: _Optional[int] = ..., draining: _Optional[bool] = ..., worker_timestamp_ms: _Optional[int] = ...) -> None: ...

class HeartbeatAck(_message.Message):
    __slots__ = ("ok", "error")
    OK_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    ok: bool
    error: str
    def __init__(self, ok: _Optional[bool] = ..., error: _Optional[str] = ...) -> None: ...

class AssignRequest(_message.Message):
    __slots__ = ("mode", "deadline_unix_ms", "expected_upload_bytes", "selected_route_id", "selected_pool_id", "selected_executor_id", "stable_egress_identity", "replayable", "attempt", "policy_version", "initial_upload_credit_bytes", "initial_download_credit_bytes", "max_inflight_upload_bytes", "max_inflight_download_bytes")
    MODE_FIELD_NUMBER: _ClassVar[int]
    DEADLINE_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_UPLOAD_BYTES_FIELD_NUMBER: _ClassVar[int]
    SELECTED_ROUTE_ID_FIELD_NUMBER: _ClassVar[int]
    SELECTED_POOL_ID_FIELD_NUMBER: _ClassVar[int]
    SELECTED_EXECUTOR_ID_FIELD_NUMBER: _ClassVar[int]
    STABLE_EGRESS_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    REPLAYABLE_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_FIELD_NUMBER: _ClassVar[int]
    POLICY_VERSION_FIELD_NUMBER: _ClassVar[int]
    INITIAL_UPLOAD_CREDIT_BYTES_FIELD_NUMBER: _ClassVar[int]
    INITIAL_DOWNLOAD_CREDIT_BYTES_FIELD_NUMBER: _ClassVar[int]
    MAX_INFLIGHT_UPLOAD_BYTES_FIELD_NUMBER: _ClassVar[int]
    MAX_INFLIGHT_DOWNLOAD_BYTES_FIELD_NUMBER: _ClassVar[int]
    mode: RequestMode
    deadline_unix_ms: int
    expected_upload_bytes: int
    selected_route_id: str
    selected_pool_id: str
    selected_executor_id: str
    stable_egress_identity: str
    replayable: bool
    attempt: int
    policy_version: str
    initial_upload_credit_bytes: int
    initial_download_credit_bytes: int
    max_inflight_upload_bytes: int
    max_inflight_download_bytes: int
    def __init__(self, mode: _Optional[_Union[RequestMode, str]] = ..., deadline_unix_ms: _Optional[int] = ..., expected_upload_bytes: _Optional[int] = ..., selected_route_id: _Optional[str] = ..., selected_pool_id: _Optional[str] = ..., selected_executor_id: _Optional[str] = ..., stable_egress_identity: _Optional[str] = ..., replayable: _Optional[bool] = ..., attempt: _Optional[int] = ..., policy_version: _Optional[str] = ..., initial_upload_credit_bytes: _Optional[int] = ..., initial_download_credit_bytes: _Optional[int] = ..., max_inflight_upload_bytes: _Optional[int] = ..., max_inflight_download_bytes: _Optional[int] = ...) -> None: ...

class AssignAck(_message.Message):
    __slots__ = ("code", "error")
    CODE_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    code: AssignAckCode
    error: str
    def __init__(self, code: _Optional[_Union[AssignAckCode, str]] = ..., error: _Optional[str] = ...) -> None: ...

class StreamFrame(_message.Message):
    __slots__ = ("stream_seq", "attempt", "request_start", "outbound_start", "response_start", "data", "credit", "body_ref", "cancel", "error", "trailers", "end", "cancelled")
    STREAM_SEQ_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_FIELD_NUMBER: _ClassVar[int]
    REQUEST_START_FIELD_NUMBER: _ClassVar[int]
    OUTBOUND_START_FIELD_NUMBER: _ClassVar[int]
    RESPONSE_START_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    CREDIT_FIELD_NUMBER: _ClassVar[int]
    BODY_REF_FIELD_NUMBER: _ClassVar[int]
    CANCEL_FIELD_NUMBER: _ClassVar[int]
    ERROR_FIELD_NUMBER: _ClassVar[int]
    TRAILERS_FIELD_NUMBER: _ClassVar[int]
    END_FIELD_NUMBER: _ClassVar[int]
    CANCELLED_FIELD_NUMBER: _ClassVar[int]
    stream_seq: int
    attempt: int
    request_start: RequestStart
    outbound_start: OutboundStartFrame
    response_start: ResponseStart
    data: DataFrame
    credit: CreditFrame
    body_ref: BodyRefFrame
    cancel: CancelFrame
    error: ErrorFrame
    trailers: TrailersFrame
    end: EndFrame
    cancelled: CancelledFrame
    def __init__(self, stream_seq: _Optional[int] = ..., attempt: _Optional[int] = ..., request_start: _Optional[_Union[RequestStart, _Mapping]] = ..., outbound_start: _Optional[_Union[OutboundStartFrame, _Mapping]] = ..., response_start: _Optional[_Union[ResponseStart, _Mapping]] = ..., data: _Optional[_Union[DataFrame, _Mapping]] = ..., credit: _Optional[_Union[CreditFrame, _Mapping]] = ..., body_ref: _Optional[_Union[BodyRefFrame, _Mapping]] = ..., cancel: _Optional[_Union[CancelFrame, _Mapping]] = ..., error: _Optional[_Union[ErrorFrame, _Mapping]] = ..., trailers: _Optional[_Union[TrailersFrame, _Mapping]] = ..., end: _Optional[_Union[EndFrame, _Mapping]] = ..., cancelled: _Optional[_Union[CancelledFrame, _Mapping]] = ...) -> None: ...

class Header(_message.Message):
    __slots__ = ("name", "value")
    NAME_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    name: str
    value: bytes
    def __init__(self, name: _Optional[str] = ..., value: _Optional[bytes] = ...) -> None: ...

class DataFrame(_message.Message):
    __slots__ = ("offset", "data")
    OFFSET_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    offset: int
    data: bytes
    def __init__(self, offset: _Optional[int] = ..., data: _Optional[bytes] = ...) -> None: ...

class CreditFrame(_message.Message):
    __slots__ = ("upload_credit_bytes", "download_credit_bytes")
    UPLOAD_CREDIT_BYTES_FIELD_NUMBER: _ClassVar[int]
    DOWNLOAD_CREDIT_BYTES_FIELD_NUMBER: _ClassVar[int]
    upload_credit_bytes: int
    download_credit_bytes: int
    def __init__(self, upload_credit_bytes: _Optional[int] = ..., download_credit_bytes: _Optional[int] = ...) -> None: ...

class ErrorFrame(_message.Message):
    __slots__ = ("code", "category", "message", "retryable", "retry_after_ms", "upstream_status", "timeout_type", "details")
    class DetailsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    CODE_FIELD_NUMBER: _ClassVar[int]
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    RETRYABLE_FIELD_NUMBER: _ClassVar[int]
    RETRY_AFTER_MS_FIELD_NUMBER: _ClassVar[int]
    UPSTREAM_STATUS_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_TYPE_FIELD_NUMBER: _ClassVar[int]
    DETAILS_FIELD_NUMBER: _ClassVar[int]
    code: ErrorCode
    category: ErrorCategory
    message: str
    retryable: bool
    retry_after_ms: int
    upstream_status: int
    timeout_type: TimeoutType
    details: _containers.ScalarMap[str, str]
    def __init__(self, code: _Optional[_Union[ErrorCode, str]] = ..., category: _Optional[_Union[ErrorCategory, str]] = ..., message: _Optional[str] = ..., retryable: _Optional[bool] = ..., retry_after_ms: _Optional[int] = ..., upstream_status: _Optional[int] = ..., timeout_type: _Optional[_Union[TimeoutType, str]] = ..., details: _Optional[_Mapping[str, str]] = ...) -> None: ...

class EndFrame(_message.Message):
    __slots__ = ("success",)
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    def __init__(self, success: _Optional[bool] = ...) -> None: ...

class CancelFrame(_message.Message):
    __slots__ = ("reason",)
    REASON_FIELD_NUMBER: _ClassVar[int]
    reason: str
    def __init__(self, reason: _Optional[str] = ...) -> None: ...

class CancelledFrame(_message.Message):
    __slots__ = ("reason",)
    REASON_FIELD_NUMBER: _ClassVar[int]
    reason: str
    def __init__(self, reason: _Optional[str] = ...) -> None: ...

class TrailersFrame(_message.Message):
    __slots__ = ("headers",)
    HEADERS_FIELD_NUMBER: _ClassVar[int]
    headers: _containers.RepeatedCompositeFieldContainer[Header]
    def __init__(self, headers: _Optional[_Iterable[_Union[Header, _Mapping]]] = ...) -> None: ...

class InjectionOperation(_message.Message):
    __slots__ = ("op", "header_name", "value")
    OP_FIELD_NUMBER: _ClassVar[int]
    HEADER_NAME_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    op: str
    header_name: str
    value: bytes
    def __init__(self, op: _Optional[str] = ..., header_name: _Optional[str] = ..., value: _Optional[bytes] = ...) -> None: ...

class DestinationPolicy(_message.Message):
    __slots__ = ("allow_private_ranges", "allow_loopback", "allow_link_local", "allow_multicast", "allow_metadata_ips", "denied_cidrs", "allowed_cidrs", "denied_host_suffixes", "denied_cname_suffixes", "sni_host_mismatch_policy", "redirect_policy", "policy_version", "resolution_mode")
    ALLOW_PRIVATE_RANGES_FIELD_NUMBER: _ClassVar[int]
    ALLOW_LOOPBACK_FIELD_NUMBER: _ClassVar[int]
    ALLOW_LINK_LOCAL_FIELD_NUMBER: _ClassVar[int]
    ALLOW_MULTICAST_FIELD_NUMBER: _ClassVar[int]
    ALLOW_METADATA_IPS_FIELD_NUMBER: _ClassVar[int]
    DENIED_CIDRS_FIELD_NUMBER: _ClassVar[int]
    ALLOWED_CIDRS_FIELD_NUMBER: _ClassVar[int]
    DENIED_HOST_SUFFIXES_FIELD_NUMBER: _ClassVar[int]
    DENIED_CNAME_SUFFIXES_FIELD_NUMBER: _ClassVar[int]
    SNI_HOST_MISMATCH_POLICY_FIELD_NUMBER: _ClassVar[int]
    REDIRECT_POLICY_FIELD_NUMBER: _ClassVar[int]
    POLICY_VERSION_FIELD_NUMBER: _ClassVar[int]
    RESOLUTION_MODE_FIELD_NUMBER: _ClassVar[int]
    allow_private_ranges: bool
    allow_loopback: bool
    allow_link_local: bool
    allow_multicast: bool
    allow_metadata_ips: bool
    denied_cidrs: _containers.RepeatedScalarFieldContainer[str]
    allowed_cidrs: _containers.RepeatedScalarFieldContainer[str]
    denied_host_suffixes: _containers.RepeatedScalarFieldContainer[str]
    denied_cname_suffixes: _containers.RepeatedScalarFieldContainer[str]
    sni_host_mismatch_policy: SniHostMismatchPolicy
    redirect_policy: RedirectPolicy
    policy_version: str
    resolution_mode: DestinationResolutionMode
    def __init__(self, allow_private_ranges: _Optional[bool] = ..., allow_loopback: _Optional[bool] = ..., allow_link_local: _Optional[bool] = ..., allow_multicast: _Optional[bool] = ..., allow_metadata_ips: _Optional[bool] = ..., denied_cidrs: _Optional[_Iterable[str]] = ..., allowed_cidrs: _Optional[_Iterable[str]] = ..., denied_host_suffixes: _Optional[_Iterable[str]] = ..., denied_cname_suffixes: _Optional[_Iterable[str]] = ..., sni_host_mismatch_policy: _Optional[_Union[SniHostMismatchPolicy, str]] = ..., redirect_policy: _Optional[_Union[RedirectPolicy, str]] = ..., policy_version: _Optional[str] = ..., resolution_mode: _Optional[_Union[DestinationResolutionMode, str]] = ...) -> None: ...

class BodyRefFrame(_message.Message):
    __slots__ = ("s3", "direct_stream", "expected_size_bytes", "sha256_hex")
    S3_FIELD_NUMBER: _ClassVar[int]
    DIRECT_STREAM_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_SIZE_BYTES_FIELD_NUMBER: _ClassVar[int]
    SHA256_HEX_FIELD_NUMBER: _ClassVar[int]
    s3: S3BodyRef
    direct_stream: DirectStreamRef
    expected_size_bytes: int
    sha256_hex: str
    def __init__(self, s3: _Optional[_Union[S3BodyRef, _Mapping]] = ..., direct_stream: _Optional[_Union[DirectStreamRef, _Mapping]] = ..., expected_size_bytes: _Optional[int] = ..., sha256_hex: _Optional[str] = ...) -> None: ...

class S3BodyRef(_message.Message):
    __slots__ = ("object_key", "signed_url", "expires_unix_ms")
    OBJECT_KEY_FIELD_NUMBER: _ClassVar[int]
    SIGNED_URL_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    object_key: str
    signed_url: str
    expires_unix_ms: int
    def __init__(self, object_key: _Optional[str] = ..., signed_url: _Optional[str] = ..., expires_unix_ms: _Optional[int] = ...) -> None: ...

class DirectStreamRef(_message.Message):
    __slots__ = ("endpoint", "stream_id", "expires_unix_ms")
    ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    STREAM_ID_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    endpoint: str
    stream_id: str
    expires_unix_ms: int
    def __init__(self, endpoint: _Optional[str] = ..., stream_id: _Optional[str] = ..., expires_unix_ms: _Optional[int] = ...) -> None: ...

class RequestStart(_message.Message):
    __slots__ = ("mode", "method", "url", "headers", "routing_metadata", "selected_route_id", "selected_pool_id", "deadline_unix_ms", "replayable", "payload_capture_decision", "fingerprint_instruction", "injection_operations", "redirect_policy", "destination_policy", "policy_version")
    class RoutingMetadataEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    MODE_FIELD_NUMBER: _ClassVar[int]
    METHOD_FIELD_NUMBER: _ClassVar[int]
    URL_FIELD_NUMBER: _ClassVar[int]
    HEADERS_FIELD_NUMBER: _ClassVar[int]
    ROUTING_METADATA_FIELD_NUMBER: _ClassVar[int]
    SELECTED_ROUTE_ID_FIELD_NUMBER: _ClassVar[int]
    SELECTED_POOL_ID_FIELD_NUMBER: _ClassVar[int]
    DEADLINE_UNIX_MS_FIELD_NUMBER: _ClassVar[int]
    REPLAYABLE_FIELD_NUMBER: _ClassVar[int]
    PAYLOAD_CAPTURE_DECISION_FIELD_NUMBER: _ClassVar[int]
    FINGERPRINT_INSTRUCTION_FIELD_NUMBER: _ClassVar[int]
    INJECTION_OPERATIONS_FIELD_NUMBER: _ClassVar[int]
    REDIRECT_POLICY_FIELD_NUMBER: _ClassVar[int]
    DESTINATION_POLICY_FIELD_NUMBER: _ClassVar[int]
    POLICY_VERSION_FIELD_NUMBER: _ClassVar[int]
    mode: RequestMode
    method: str
    url: str
    headers: _containers.RepeatedCompositeFieldContainer[Header]
    routing_metadata: _containers.ScalarMap[str, str]
    selected_route_id: str
    selected_pool_id: str
    deadline_unix_ms: int
    replayable: bool
    payload_capture_decision: str
    fingerprint_instruction: str
    injection_operations: _containers.RepeatedCompositeFieldContainer[InjectionOperation]
    redirect_policy: RedirectPolicy
    destination_policy: DestinationPolicy
    policy_version: str
    def __init__(self, mode: _Optional[_Union[RequestMode, str]] = ..., method: _Optional[str] = ..., url: _Optional[str] = ..., headers: _Optional[_Iterable[_Union[Header, _Mapping]]] = ..., routing_metadata: _Optional[_Mapping[str, str]] = ..., selected_route_id: _Optional[str] = ..., selected_pool_id: _Optional[str] = ..., deadline_unix_ms: _Optional[int] = ..., replayable: _Optional[bool] = ..., payload_capture_decision: _Optional[str] = ..., fingerprint_instruction: _Optional[str] = ..., injection_operations: _Optional[_Iterable[_Union[InjectionOperation, _Mapping]]] = ..., redirect_policy: _Optional[_Union[RedirectPolicy, str]] = ..., destination_policy: _Optional[_Union[DestinationPolicy, _Mapping]] = ..., policy_version: _Optional[str] = ...) -> None: ...

class OutboundStartFrame(_message.Message):
    __slots__ = ("target_host", "target_port", "upstream_proxy_id", "attempt", "worker_timestamp_ms")
    TARGET_HOST_FIELD_NUMBER: _ClassVar[int]
    TARGET_PORT_FIELD_NUMBER: _ClassVar[int]
    UPSTREAM_PROXY_ID_FIELD_NUMBER: _ClassVar[int]
    ATTEMPT_FIELD_NUMBER: _ClassVar[int]
    WORKER_TIMESTAMP_MS_FIELD_NUMBER: _ClassVar[int]
    target_host: str
    target_port: int
    upstream_proxy_id: str
    attempt: int
    worker_timestamp_ms: int
    def __init__(self, target_host: _Optional[str] = ..., target_port: _Optional[int] = ..., upstream_proxy_id: _Optional[str] = ..., attempt: _Optional[int] = ..., worker_timestamp_ms: _Optional[int] = ...) -> None: ...

class ResponseStart(_message.Message):
    __slots__ = ("status", "headers")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    HEADERS_FIELD_NUMBER: _ClassVar[int]
    status: int
    headers: _containers.RepeatedCompositeFieldContainer[Header]
    def __init__(self, status: _Optional[int] = ..., headers: _Optional[_Iterable[_Union[Header, _Mapping]]] = ...) -> None: ...

class ErrorResponse(_message.Message):
    __slots__ = ("category", "code", "message", "retryable", "retry_after_ms", "request_id", "upstream_status", "timeout_type", "details")
    class DetailsEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    CATEGORY_FIELD_NUMBER: _ClassVar[int]
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    RETRYABLE_FIELD_NUMBER: _ClassVar[int]
    RETRY_AFTER_MS_FIELD_NUMBER: _ClassVar[int]
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    UPSTREAM_STATUS_FIELD_NUMBER: _ClassVar[int]
    TIMEOUT_TYPE_FIELD_NUMBER: _ClassVar[int]
    DETAILS_FIELD_NUMBER: _ClassVar[int]
    category: ErrorCategory
    code: ErrorCode
    message: str
    retryable: bool
    retry_after_ms: int
    request_id: str
    upstream_status: int
    timeout_type: TimeoutType
    details: _containers.ScalarMap[str, str]
    def __init__(self, category: _Optional[_Union[ErrorCategory, str]] = ..., code: _Optional[_Union[ErrorCode, str]] = ..., message: _Optional[str] = ..., retryable: _Optional[bool] = ..., retry_after_ms: _Optional[int] = ..., request_id: _Optional[str] = ..., upstream_status: _Optional[int] = ..., timeout_type: _Optional[_Union[TimeoutType, str]] = ..., details: _Optional[_Mapping[str, str]] = ...) -> None: ...

package strawpb

import (
	"errors"
)

var (
	errAssignRequestNil             = errors.New("assign request is nil")
	errAssignRequestModeInvalid     = errors.New("invalid assign request mode")
	errDestinationPolicyNil         = errors.New("destination policy is nil")
	errSNIHostMismatchPolicyInvalid = errors.New("invalid sni host mismatch policy")
	errRedirectPolicyInvalid        = errors.New("invalid redirect policy")
	errDestinationResolutionInvalid = errors.New("invalid destination resolution mode")
	errRequestStartNil              = errors.New("request start is nil")
	errRequestModeInvalid           = errors.New("invalid request mode")
	errErrorResponseNil             = errors.New("error response is nil")
	errErrorCategoryInvalid         = errors.New("invalid error category")
	errErrorCodeInvalid             = errors.New("invalid error code")
	errTimeoutTypeInvalid           = errors.New("invalid timeout type")
	errHeartbeatRequestNil          = errors.New("heartbeat request is nil")
	errWorkerHealthInvalid          = errors.New("invalid worker health")
	errAssignAckNil                 = errors.New("assign ack is nil")
	errAssignAckCodeInvalid         = errors.New("invalid assign ack code")
)

// Validate checks AssignRequest fields for P0 constraints.
func (m *AssignRequest) Validate() error {
	if m == nil {
		return errAssignRequestNil
	}

	if !m.Mode.Valid() {
		return errAssignRequestModeInvalid
	}

	return nil
}

// Validate checks DestinationPolicy fields for P0 constraints.
func (m *DestinationPolicy) Validate() error {
	if m == nil {
		return errDestinationPolicyNil
	}

	if !m.SniHostMismatchPolicy.Valid() {
		return errSNIHostMismatchPolicyInvalid
	}

	if !m.RedirectPolicy.Valid() {
		return errRedirectPolicyInvalid
	}

	if !m.ResolutionMode.Valid() {
		return errDestinationResolutionInvalid
	}

	return nil
}

// Validate checks RequestStart fields for P0 constraints.
func (m *RequestStart) Validate() error {
	if m == nil {
		return errRequestStartNil
	}

	if !m.Mode.Valid() {
		return errRequestModeInvalid
	}

	if !m.RedirectPolicy.Valid() {
		return errRedirectPolicyInvalid
	}

	return m.DestinationPolicy.Validate()
}

// Validate checks ErrorResponse fields for P0 constraints.
func (m *ErrorResponse) Validate() error {
	if m == nil {
		return errErrorResponseNil
	}

	if !m.Category.Valid() {
		return errErrorCategoryInvalid
	}

	if !m.Code.Valid() {
		return errErrorCodeInvalid
	}

	if m.TimeoutType != nil && !m.TimeoutType.Valid() {
		return errTimeoutTypeInvalid
	}

	return nil
}

// Validate checks HeartbeatRequest fields for P0 constraints.
func (m *HeartbeatRequest) Validate() error {
	if m == nil {
		return errHeartbeatRequestNil
	}

	if !m.Health.Valid() {
		return errWorkerHealthInvalid
	}

	return nil
}

// Validate checks AssignAck fields for P0 constraints.
func (m *AssignAck) Validate() error {
	if m == nil {
		return errAssignAckNil
	}

	if !m.Code.Valid() {
		return errAssignAckCodeInvalid
	}

	return nil
}

// Valid reports whether the error category is known.
func (e ErrorCategory) Valid() bool {
	switch e {
	case ErrorCategory_ERROR_CATEGORY_UNSPECIFIED,
		ErrorCategory_ERROR_CATEGORY_CLIENT,
		ErrorCategory_ERROR_CATEGORY_ROUTING,
		ErrorCategory_ERROR_CATEGORY_TRANSPORT,
		ErrorCategory_ERROR_CATEGORY_EGRESS,
		ErrorCategory_ERROR_CATEGORY_STREAMING,
		ErrorCategory_ERROR_CATEGORY_CONTROL:
		return true
	default:
		return false
	}
}

// Valid reports whether the error code is known.
func (e ErrorCode) Valid() bool {
	switch e {
	case ErrorCode_ERROR_CODE_UNSPECIFIED,
		ErrorCode_ERROR_CODE_AUTH_FAILURE,
		ErrorCode_ERROR_CODE_TENANT_NOT_FOUND,
		ErrorCode_ERROR_CODE_INSUFFICIENT_PERMISSIONS,
		ErrorCode_ERROR_CODE_RATE_LIMIT_EXCEEDED,
		ErrorCode_ERROR_CODE_QUOTA_EXHAUSTED,
		ErrorCode_ERROR_CODE_INVALID_REQUEST,
		ErrorCode_ERROR_CODE_DESTINATION_DENIED,
		ErrorCode_ERROR_CODE_HEADER_INJECTION_FAILED,
		ErrorCode_ERROR_CODE_CONFLICT,
		ErrorCode_ERROR_CODE_UNSUPPORTED_INGRESS_MODE,
		ErrorCode_ERROR_CODE_ROUTE_NO_MATCH,
		ErrorCode_ERROR_CODE_ROUTE_UNAVAILABLE,
		ErrorCode_ERROR_CODE_STICKY_SESSION_UNAVAILABLE,
		ErrorCode_ERROR_CODE_EXECUTOR_CAPACITY_EXHAUSTED,
		ErrorCode_ERROR_CODE_ASSIGNMENT_TIMEOUT,
		ErrorCode_ERROR_CODE_WORKER_DISCONNECTED,
		ErrorCode_ERROR_CODE_TRANSPORT_UNAVAILABLE,
		ErrorCode_ERROR_CODE_PROTOCOL_ERROR,
		ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED,
		ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT,
		ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE,
		ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE,
		ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED,
		ErrorCode_ERROR_CODE_UPSTREAM_CONNECT_TIMEOUT,
		ErrorCode_ERROR_CODE_UPSTREAM_RESET,
		ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE,
		ErrorCode_ERROR_CODE_STREAM_UPLOAD_ABORTED,
		ErrorCode_ERROR_CODE_STREAM_DOWNLOAD_ABORTED,
		ErrorCode_ERROR_CODE_BODY_REF_UNAVAILABLE,
		ErrorCode_ERROR_CODE_BODY_TOO_LARGE,
		ErrorCode_ERROR_CODE_CONTROL_INTERNAL_ERROR,
		ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR,
		ErrorCode_ERROR_CODE_CANCELLED:
		return true
	default:
		return false
	}
}

// Valid reports whether the timeout type is known.
func (e TimeoutType) Valid() bool {
	switch e {
	case TimeoutType_TIMEOUT_TYPE_UNSPECIFIED,
		TimeoutType_TIMEOUT_TYPE_ASSIGNMENT_TIMEOUT,
		TimeoutType_TIMEOUT_TYPE_CONNECT_TIMEOUT,
		TimeoutType_TIMEOUT_TYPE_RESPONSE_HEADER_TIMEOUT,
		TimeoutType_TIMEOUT_TYPE_IDLE_TIMEOUT,
		TimeoutType_TIMEOUT_TYPE_UPLOAD_TIMEOUT,
		TimeoutType_TIMEOUT_TYPE_DOWNLOAD_TIMEOUT,
		TimeoutType_TIMEOUT_TYPE_TOTAL_DEADLINE_TIMEOUT:
		return true
	default:
		return false
	}
}

// Valid reports whether the assign ack code is known.
func (e AssignAckCode) Valid() bool {
	switch e {
	case AssignAckCode_ASSIGN_ACK_CODE_UNSPECIFIED,
		AssignAckCode_ASSIGN_ACK_ACCEPTED,
		AssignAckCode_ASSIGN_ACK_REJECTED_CAPACITY,
		AssignAckCode_ASSIGN_ACK_REJECTED_DRAINING,
		AssignAckCode_ASSIGN_ACK_REJECTED_UNSUPPORTED,
		AssignAckCode_ASSIGN_ACK_REJECTED_AUTH_SCOPE,
		AssignAckCode_ASSIGN_ACK_REJECTED_ERROR:
		return true
	default:
		return false
	}
}

// Valid reports whether the request mode is known.
func (e RequestMode) Valid() bool {
	switch e {
	case RequestMode_REQUEST_MODE_UNSPECIFIED,
		RequestMode_REQUEST_MODE_DECODED_HTTP,
		RequestMode_REQUEST_MODE_RAW_TUNNEL:
		return true
	default:
		return false
	}
}

// Valid reports whether the worker health value is known.
func (e WorkerHealth) Valid() bool {
	switch e {
	case WorkerHealth_WORKER_HEALTH_UNSPECIFIED,
		WorkerHealth_WORKER_HEALTH_READY,
		WorkerHealth_WORKER_HEALTH_DEGRADED,
		WorkerHealth_WORKER_HEALTH_UNHEALTHY:
		return true
	default:
		return false
	}
}

// Valid reports whether the SNI host mismatch policy is known.
func (e SniHostMismatchPolicy) Valid() bool {
	switch e {
	case SniHostMismatchPolicy_SNI_HOST_MISMATCH_STRICT,
		SniHostMismatchPolicy_SNI_HOST_MISMATCH_WARN,
		SniHostMismatchPolicy_SNI_HOST_MISMATCH_ALLOW:
		return true
	default:
		return false
	}
}

// Valid reports whether the redirect policy is known.
func (e RedirectPolicy) Valid() bool {
	switch e {
	case RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		RedirectPolicy_REDIRECT_POLICY_FOLLOW_STRICT:
		return true
	default:
		return false
	}
}

// Valid reports whether the destination resolution mode is known.
func (e DestinationResolutionMode) Valid() bool {
	switch e {
	case DestinationResolutionMode_DESTINATION_RESOLUTION_MODE_UNSPECIFIED,
		DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL,
		DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE,
		DestinationResolutionMode_DESTINATION_RESOLUTION_EXECUTOR_DELEGATED:
		return true
	default:
		return false
	}
}

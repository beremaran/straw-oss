package strawpb

import "fmt"

func (m *AssignRequest) Validate() error {
	if m == nil {
		return fmt.Errorf("assign request is nil")
	}
	if !m.Mode.Valid() {
		return fmt.Errorf("invalid assign request mode: %d", m.Mode)
	}
	return nil
}

func (m *DestinationPolicy) Validate() error {
	if m == nil {
		return fmt.Errorf("destination policy is nil")
	}
	if !m.SniHostMismatchPolicy.Valid() {
		return fmt.Errorf("invalid sni host mismatch policy: %d", m.SniHostMismatchPolicy)
	}
	if !m.RedirectPolicy.Valid() {
		return fmt.Errorf("invalid redirect policy: %d", m.RedirectPolicy)
	}
	if !m.ResolutionMode.Valid() {
		return fmt.Errorf("invalid destination resolution mode: %d", m.ResolutionMode)
	}
	return nil
}

func (m *RequestStart) Validate() error {
	if m == nil {
		return fmt.Errorf("request start is nil")
	}
	if !m.Mode.Valid() {
		return fmt.Errorf("invalid request mode: %d", m.Mode)
	}
	if !m.RedirectPolicy.Valid() {
		return fmt.Errorf("invalid redirect policy: %d", m.RedirectPolicy)
	}
	return m.DestinationPolicy.Validate()
}

func (m *ErrorResponse) Validate() error {
	if m == nil {
		return fmt.Errorf("error response is nil")
	}
	if !m.Category.Valid() {
		return fmt.Errorf("invalid error category: %d", m.Category)
	}
	if !m.Code.Valid() {
		return fmt.Errorf("invalid error code: %d", m.Code)
	}
	if m.TimeoutType != nil && !m.TimeoutType.Valid() {
		return fmt.Errorf("invalid timeout type: %d", *m.TimeoutType)
	}
	return nil
}

func (m *HeartbeatRequest) Validate() error {
	if m == nil {
		return fmt.Errorf("heartbeat request is nil")
	}
	if !m.Health.Valid() {
		return fmt.Errorf("invalid worker health: %d", m.Health)
	}
	return nil
}

func (m *AssignAck) Validate() error {
	if m == nil {
		return fmt.Errorf("assign ack is nil")
	}
	if !m.Code.Valid() {
		return fmt.Errorf("invalid assign ack code: %d", m.Code)
	}
	return nil
}

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

func (e RedirectPolicy) Valid() bool {
	switch e {
	case RedirectPolicy_REDIRECT_POLICY_NO_FOLLOW,
		RedirectPolicy_REDIRECT_POLICY_FOLLOW_STRICT:
		return true
	default:
		return false
	}
}

func (e DestinationResolutionMode) Valid() bool {
	switch e {
	case DestinationResolutionMode_DESTINATION_RESOLUTION_MODE_UNSPECIFIED,
		DestinationResolutionMode_DESTINATION_RESOLUTION_DIRECT_LOCAL,
		DestinationResolutionMode_DESTINATION_RESOLUTION_UPSTREAM_PROXY_REMOTE,
		DestinationResolutionMode_DESTINATION_RESOLUTION_PROVIDER_ADAPTER:
		return true
	default:
		return false
	}
}

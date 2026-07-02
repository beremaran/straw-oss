package control

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the public error envelope per the canonical error registry.
type ErrorResponse struct {
	Category    string            `json:"category"`
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	Retryable   bool              `json:"retryable"`
	RequestID   string            `json:"request_id"`
	TimeoutType string            `json:"timeout_type,omitempty"`
	Details     map[string]string `json:"details,omitempty"`
}

// ErrorCode represents a canonical error code from the error registry.
type ErrorCode int

const (
	AuthFailure ErrorCode = 1
	// ... tenant_not_found, insufficient_permissions, etc.
)

// ErrorRegistry maps ErrorCode to ErrorResponse fields.
var ErrorRegistry = map[ErrorCode]ErrorEntry{
	1:   {Category: "client", Code: "auth_failure", Message: "Invalid API key", Retryable: false, HTTPStatus: 401},
	2:   {Category: "client", Code: "tenant_not_found", Message: "Key references missing or deleted tenant", Retryable: false, HTTPStatus: 401},
	3:   {Category: "client", Code: "insufficient_permissions", Message: "RBAC failure", Retryable: false, HTTPStatus: 403},
	4:   {Category: "client", Code: "rate_limit_exceeded", Message: "Rate limit exceeded", Retryable: true, HTTPStatus: 429},
	5:   {Category: "client", Code: "quota_exhausted", Message: "Quota exhausted", Retryable: true, HTTPStatus: 429},
	6:   {Category: "client", Code: "invalid_request", Message: "Malformed request or missing business fields", Retryable: false, HTTPStatus: 400},
	7:   {Category: "client", Code: "destination_denied", Message: "Deny rule matched", Retryable: false, HTTPStatus: 403},
	9:   {Category: "client", Code: "conflict", Message: "Config version conflict", Retryable: false, HTTPStatus: 409},
	10:  {Category: "client", Code: "unsupported_ingress_mode", Message: "Unsupported mode for endpoint or route", Retryable: false, HTTPStatus: 400},
	100: {Category: "routing", Code: "route_no_match", Message: "No rule matched", Retryable: false, HTTPStatus: 404},
	101: {Category: "routing", Code: "route_unavailable", Message: "Rule matched but no eligible executor", Retryable: true, HTTPStatus: 503},
	102: {Category: "routing", Code: "sticky_session_unavailable", Message: "Sticky target unavailable and fallback not allowed", Retryable: false, HTTPStatus: 503},
	103: {Category: "routing", Code: "executor_capacity_exhausted", Message: "All eligible executors at capacity", Retryable: true, HTTPStatus: 503},
	200: {Category: "transport", Code: "assignment_timeout", Message: "No AssignAck before timeout", Retryable: true, HTTPStatus: 504},
	201: {Category: "transport", Code: "worker_disconnected", Message: "Worker lost mid-request", Retryable: true, HTTPStatus: 502},
	202: {Category: "transport", Code: "transport_unavailable", Message: "NATS unavailable", Retryable: true, HTTPStatus: 504},
	203: {Category: "transport", Code: "protocol_error", Message: "Invalid frame or sequence", Retryable: false, HTTPStatus: 502},
	204: {Category: "transport", Code: "timeout_exceeded", Message: "Request deadline exceeded", Retryable: false, HTTPStatus: 504},
	205: {Category: "transport", Code: "unsupported_fingerprint", Message: "Executor cannot apply requested preset", Retryable: false, HTTPStatus: 400},
	300: {Category: "egress", Code: "upstream_dns_failure", Message: "DNS resolution failed for target host", Retryable: true, HTTPStatus: 502},
	301: {Category: "egress", Code: "upstream_tls_failure", Message: "TLS handshake or certificate failure", Retryable: true, HTTPStatus: 502},
	302: {Category: "egress", Code: "upstream_connection_refused", Message: "Connection refused", Retryable: true, HTTPStatus: 502},
	303: {Category: "egress", Code: "upstream_connect_timeout", Message: "Could not connect before timeout", Retryable: true, HTTPStatus: 504},
	304: {Category: "egress", Code: "upstream_reset", Message: "Upstream closed or reset before complete response", Retryable: true, HTTPStatus: 502},
	305: {Category: "egress", Code: "upstream_proxy_failure", Message: "Configured upstream proxy failed", Retryable: true, HTTPStatus: 502},
	400: {Category: "streaming", Code: "stream_upload_aborted", Message: "Upload interrupted", Retryable: false, HTTPStatus: 502},
	401: {Category: "streaming", Code: "stream_download_aborted", Message: "Download interrupted", Retryable: false, HTTPStatus: 502},
	402: {Category: "streaming", Code: "body_ref_unavailable", Message: "BodyRef object unavailable", Retryable: true, HTTPStatus: 502},
	403: {Category: "streaming", Code: "body_too_large", Message: "Request or response exceeds configured limit", Retryable: false, HTTPStatus: 413},
	500: {Category: "control", Code: "control_internal_error", Message: "Unexpected internal failure", Retryable: false, HTTPStatus: 500},
	501: {Category: "egress", Code: "executor_internal_error", Message: "Unexpected executor failure", Retryable: false, HTTPStatus: 502},
	502: {Category: "client", Code: "cancelled", Message: "Client or admin cancellation", Retryable: false, HTTPStatus: 499},
}

// ErrorEntry holds the metadata for a canonical error code.
type ErrorEntry struct {
	Category   string `json:"category"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	HTTPStatus int    `json:"-"`
}

// ErrorCodeFromName looks up an ErrorCode by its string name.
func ErrorCodeFromName(name string) ErrorCode {
	for code, entry := range ErrorRegistry {
		if entry.Code == name {
			return code
		}
	}
	return 0
}

// ErrorResponseFromCode builds an ErrorResponse for a canonical code.
func ErrorResponseFromCode(code ErrorCode, requestID string, extraDetails map[string]string) ErrorResponse {
	entry, ok := ErrorRegistry[code]
	if !ok {
		entry = ErrorEntry{
			Category:   "control",
			Code:       "control_internal_error",
			Message:    "Unknown error code",
			Retryable:  false,
			HTTPStatus: 500,
		}
	}
	resp := ErrorResponse{
		Category:  entry.Category,
		Code:      entry.Code,
		Message:   entry.Message,
		Retryable: entry.Retryable,
		RequestID: requestID,
	}
	if len(extraDetails) > 0 {
		resp.Details = extraDetails
	}
	return resp
}

// WriteError writes an ErrorResponse as JSON with the given HTTP status.
func WriteError(w http.ResponseWriter, status int, err ErrorResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(err)
}

// WriteValidationError writes a ValidationError as JSON with the appropriate status.
func WriteValidationError(w http.ResponseWriter, requestID string, verr *ValidationError) {
	WriteError(w, verr.HTTPStatus(), ErrorResponse{
		Category:  "client",
		Code:      verr.Code,
		Message:   verr.Message,
		Retryable: false,
		RequestID: requestID,
		Details:   verr.Details,
	})
}

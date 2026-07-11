package control

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the public error envelope per the canonical error registry.
type ErrorResponse struct {
	Category     string            `json:"category"`
	Code         string            `json:"code"`
	Message      string            `json:"message"`
	Retryable    bool              `json:"retryable"`
	RequestID    string            `json:"request_id"`
	TimeoutType  string            `json:"timeout_type,omitempty"`
	RetryAfterMs int64             `json:"retry_after_ms,omitempty"`
	Details      map[string]string `json:"details,omitempty"`
}

// ErrorCode represents a canonical error code from the error registry.
type ErrorCode int

const (
	statusBadRequest            = http.StatusBadRequest
	statusUnauthorized          = http.StatusUnauthorized
	statusForbidden             = http.StatusForbidden
	statusNotFound              = http.StatusNotFound
	statusTooManyRequests       = http.StatusTooManyRequests
	statusInternalServerError   = http.StatusInternalServerError
	statusBadGateway            = http.StatusBadGateway
	statusServiceUnavailable    = http.StatusServiceUnavailable
	statusGatewayTimeout        = http.StatusGatewayTimeout
	statusRequestEntityTooLarge = http.StatusRequestEntityTooLarge
	statusTooEarly              = http.StatusTooEarly
	statusClientClosedRequest   = 499

	// errorDetailReasonKey is the ErrorResponse.Details reason key.
	// to explain an invalid_request rejection.
	errorDetailReasonKey     = "reason"
	errorDetailDirectionKey  = "direction"
	errorDetailLimitBytesKey = "limit_bytes"

	errorCategoryClient    = "client"
	errorCategoryRouting   = "routing"
	errorCategoryTransport = "transport"
	errorCategoryEgress    = "egress"
	errorCategoryStreaming = "streaming"
	errorCategoryControl   = "control"

	errorCodeAuthFailure               = "auth_failure"
	errorCodeInvalidRequest            = "invalid_request"
	errorCodeDestinationDenied         = "destination_denied"
	errorCodeHeaderInjectionFailed     = "header_injection_failed"
	errorCodeUnsupportedIngressMode    = "unsupported_ingress_mode"
	errorCodeRouteNoMatch              = "route_no_match"
	errorCodeRouteUnavailable          = "route_unavailable"
	errorCodeStickySessionUnavailable  = "sticky_session_unavailable"
	errorCodeExecutorCapacityExhausted = "executor_capacity_exhausted"
	errorCodeAssignmentTimeout         = "assignment_timeout"
	errorCodeWorkerDisconnected        = "worker_disconnected"
	errorCodeTransportUnavailable      = "transport_unavailable"
	errorCodeProtocolError             = "protocol_error"
	errorCodeTimeoutExceeded           = "timeout_exceeded"
	errorCodeUnsupportedFingerprint    = "unsupported_fingerprint"
	errorCodeUpstreamDNSFailure        = "upstream_dns_failure"
	errorCodeUpstreamTLSFailure        = "upstream_tls_failure"
	errorCodeUpstreamConnectionRefused = "upstream_connection_refused"
	errorCodeUpstreamConnectTimeout    = "upstream_connect_timeout"
	errorCodeUpstreamReset             = "upstream_reset"
	errorCodeUpstreamProxyFailure      = "upstream_proxy_failure"
	errorCodeStreamUploadAborted       = "stream_upload_aborted"
	errorCodeStreamDownloadAborted     = "stream_download_aborted"
	errorCodeBodyTooLarge              = "body_too_large"
	errorCodeControlInternalError      = "control_internal_error"
	errorCodeExecutorInternalError     = "executor_internal_error"
	errorCodeCancelled                 = "cancelled"
)

const (
	// AuthFailure is returned when a bearer token is invalid or revoked.
	AuthFailure ErrorCode = 1
	// InvalidRequest is returned when the request is malformed or incomplete.
	InvalidRequest ErrorCode = 6
	// DestinationDenied is returned when a deny rule matches the request.
	DestinationDenied ErrorCode = 7
	// HeaderInjectionFailed is returned when a Control-resolved injection operation is invalid.
	HeaderInjectionFailed ErrorCode = 8
	// UnsupportedIngressMode is returned when the endpoint rejects the mode.
	UnsupportedIngressMode ErrorCode = 10
	// RouteNoMatch is returned when no routing rule matches the request.
	RouteNoMatch ErrorCode = 100
	// RouteUnavailable is returned when a rule matches but no executor is eligible.
	RouteUnavailable ErrorCode = 101
	// StickySessionUnavailable is returned when a sticky target cannot be honored.
	StickySessionUnavailable ErrorCode = 102
	// ExecutorCapacityExhausted is returned when every executor is saturated.
	ExecutorCapacityExhausted ErrorCode = 103
	// AssignmentTimeout is returned when Control does not receive an assignment ack in time.
	AssignmentTimeout ErrorCode = 200
	// WorkerDisconnected is returned when the worker connection drops mid-request.
	WorkerDisconnected ErrorCode = 201
	// TransportUnavailable is returned when the NATS transport is unavailable.
	TransportUnavailable ErrorCode = 202
	// ProtocolError is returned when a frame or sequence is invalid.
	ProtocolError ErrorCode = 203
	// TimeoutExceeded is returned when a request deadline is exceeded.
	TimeoutExceeded ErrorCode = 204
	// UnsupportedFingerprint is returned when the worker cannot apply the requested preset.
	UnsupportedFingerprint ErrorCode = 205
	// UpstreamDNSFailure is returned when DNS resolution fails.
	UpstreamDNSFailure ErrorCode = 300
	// UpstreamTLSFailure is returned when TLS negotiation fails.
	UpstreamTLSFailure ErrorCode = 301
	// UpstreamConnectionRefused is returned when the upstream actively refuses the connection.
	UpstreamConnectionRefused ErrorCode = 302
	// UpstreamConnectTimeout is returned when the upstream cannot be reached in time.
	UpstreamConnectTimeout ErrorCode = 303
	// UpstreamReset is returned when the upstream resets or closes early.
	UpstreamReset ErrorCode = 304
	// UpstreamProxyFailure is returned when an upstream proxy fails.
	UpstreamProxyFailure ErrorCode = 305
	// StreamUploadAborted is returned when an upload stream is interrupted.
	StreamUploadAborted ErrorCode = 400
	// StreamDownloadAborted is returned when a download stream is interrupted.
	StreamDownloadAborted ErrorCode = 401
	// BodyTooLarge is returned when inline content exceeds the configured maximum.
	BodyTooLarge ErrorCode = 403
	// ControlInternalError is returned for unexpected control-plane failures.
	ControlInternalError ErrorCode = 500
	// ExecutorInternalError is returned for unexpected executor failures.
	ExecutorInternalError ErrorCode = 501
	// Cancelled is returned when a request is cancelled.
	Cancelled ErrorCode = 502
)

// ErrorRegistry maps ErrorCode to ErrorResponse fields.
var ErrorRegistry = map[ErrorCode]ErrorEntry{
	AuthFailure:               {Category: errorCategoryClient, Code: errorCodeAuthFailure, Message: "Invalid deployment token", Retryable: false, HTTPStatus: statusUnauthorized},
	InvalidRequest:            {Category: errorCategoryClient, Code: errorCodeInvalidRequest, Message: "Malformed request or missing business fields", Retryable: false, HTTPStatus: statusBadRequest},
	DestinationDenied:         {Category: errorCategoryClient, Code: errorCodeDestinationDenied, Message: "Deny rule matched", Retryable: false, HTTPStatus: statusForbidden},
	HeaderInjectionFailed:     {Category: errorCategoryClient, Code: errorCodeHeaderInjectionFailed, Message: "Resolved injection invalid", Retryable: false, HTTPStatus: statusBadRequest},
	UnsupportedIngressMode:    {Category: errorCategoryClient, Code: errorCodeUnsupportedIngressMode, Message: "Unsupported mode for endpoint or route", Retryable: false, HTTPStatus: statusBadRequest},
	RouteNoMatch:              {Category: errorCategoryRouting, Code: errorCodeRouteNoMatch, Message: "No rule matched", Retryable: false, HTTPStatus: statusNotFound},
	RouteUnavailable:          {Category: errorCategoryRouting, Code: errorCodeRouteUnavailable, Message: "Rule matched but no eligible executor", Retryable: true, HTTPStatus: statusServiceUnavailable},
	StickySessionUnavailable:  {Category: errorCategoryRouting, Code: errorCodeStickySessionUnavailable, Message: "Sticky target unavailable and fallback not allowed", Retryable: false, HTTPStatus: statusServiceUnavailable},
	ExecutorCapacityExhausted: {Category: errorCategoryRouting, Code: errorCodeExecutorCapacityExhausted, Message: "All eligible executors at capacity", Retryable: true, HTTPStatus: statusServiceUnavailable},
	AssignmentTimeout:         {Category: errorCategoryTransport, Code: errorCodeAssignmentTimeout, Message: "No AssignAck before timeout", Retryable: true, HTTPStatus: statusGatewayTimeout},
	WorkerDisconnected:        {Category: errorCategoryTransport, Code: errorCodeWorkerDisconnected, Message: "Worker lost mid-request", Retryable: true, HTTPStatus: statusBadGateway},
	TransportUnavailable:      {Category: errorCategoryTransport, Code: errorCodeTransportUnavailable, Message: "NATS unavailable", Retryable: true, HTTPStatus: statusGatewayTimeout},
	ProtocolError:             {Category: errorCategoryTransport, Code: errorCodeProtocolError, Message: "Invalid frame or sequence", Retryable: false, HTTPStatus: statusBadGateway},
	TimeoutExceeded:           {Category: errorCategoryTransport, Code: errorCodeTimeoutExceeded, Message: "Request deadline exceeded", Retryable: false, HTTPStatus: statusGatewayTimeout},
	UnsupportedFingerprint:    {Category: errorCategoryTransport, Code: errorCodeUnsupportedFingerprint, Message: "Executor cannot apply requested preset", Retryable: false, HTTPStatus: statusBadRequest},
	UpstreamDNSFailure:        {Category: errorCategoryEgress, Code: errorCodeUpstreamDNSFailure, Message: "DNS resolution failed for target host", Retryable: true, HTTPStatus: statusBadGateway},
	UpstreamTLSFailure:        {Category: errorCategoryEgress, Code: errorCodeUpstreamTLSFailure, Message: "TLS handshake or certificate failure", Retryable: true, HTTPStatus: statusBadGateway},
	UpstreamConnectionRefused: {Category: errorCategoryEgress, Code: errorCodeUpstreamConnectionRefused, Message: "Connection refused", Retryable: true, HTTPStatus: statusBadGateway},
	UpstreamConnectTimeout:    {Category: errorCategoryEgress, Code: errorCodeUpstreamConnectTimeout, Message: "Could not connect before timeout", Retryable: true, HTTPStatus: statusGatewayTimeout},
	UpstreamReset:             {Category: errorCategoryEgress, Code: errorCodeUpstreamReset, Message: "Upstream closed or reset before complete response", Retryable: true, HTTPStatus: statusBadGateway},
	UpstreamProxyFailure:      {Category: errorCategoryEgress, Code: errorCodeUpstreamProxyFailure, Message: "Configured upstream proxy failed", Retryable: true, HTTPStatus: statusBadGateway},
	StreamUploadAborted:       {Category: errorCategoryStreaming, Code: errorCodeStreamUploadAborted, Message: "Upload interrupted", Retryable: false, HTTPStatus: statusBadGateway},
	StreamDownloadAborted:     {Category: errorCategoryStreaming, Code: errorCodeStreamDownloadAborted, Message: "Download interrupted", Retryable: false, HTTPStatus: statusBadGateway},
	BodyTooLarge:              {Category: errorCategoryStreaming, Code: errorCodeBodyTooLarge, Message: "Request or response exceeds configured limit", Retryable: false, HTTPStatus: http.StatusRequestEntityTooLarge},
	ControlInternalError:      {Category: errorCategoryControl, Code: errorCodeControlInternalError, Message: "Unexpected internal failure", Retryable: false, HTTPStatus: statusInternalServerError},
	ExecutorInternalError:     {Category: errorCategoryEgress, Code: errorCodeExecutorInternalError, Message: "Unexpected executor failure", Retryable: false, HTTPStatus: statusBadGateway},
	Cancelled:                 {Category: errorCategoryClient, Code: errorCodeCancelled, Message: "Request cancelled", Retryable: false, HTTPStatus: statusClientClosedRequest},
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
			HTTPStatus: statusInternalServerError,
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

// ErrorResponseFromCodeWithRetry builds an ErrorResponse and includes a
// positive retry delay when one is available.
func ErrorResponseFromCodeWithRetry(code ErrorCode, requestID string, extraDetails map[string]string, retryAfterMs int64) ErrorResponse {
	resp := ErrorResponseFromCode(code, requestID, extraDetails)
	if retryAfterMs > 0 {
		resp.RetryAfterMs = retryAfterMs
	}

	return resp
}

// WriteError writes an ErrorResponse as JSON with the given HTTP status.
func WriteError(w http.ResponseWriter, status int, err ErrorResponse) {
	w.Header().Set(headerCanonicalContentType, "application/json")
	w.WriteHeader(status)
	encodeErr := json.NewEncoder(w).Encode(err)
	_ = encodeErr
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

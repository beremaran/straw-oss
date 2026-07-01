// Package protocol provides protobuf serialization helpers and shared protocol constants.
package protocol

// Error codes returned in protobuf ErrorInfo messages.
const (
	ErrCodeEgressTimeout = "EGRESS_TIMEOUT"
	ErrCodeUpstreamError = "UPSTREAM_ERROR"
	ErrCodeInternalError = "INTERNAL_ERROR"
)

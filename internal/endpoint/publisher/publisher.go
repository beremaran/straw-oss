// Package publisher provides result publishing for the Endpoint worker.
// It publishes HTTP request results back to the Relay Server via RabbitMQ.
package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// Publisher publishes request results to the message broker.
type Publisher struct {
	broker broker.MessageBroker
	logger *slog.Logger

	// useConfirm enables publisher confirms for reliable delivery.
	useConfirm bool
}

// Option is a functional option for configuring the Publisher.
type Option func(*Publisher)

// WithLogger sets the logger for the publisher.
func WithLogger(logger *slog.Logger) Option {
	return func(p *Publisher) {
		p.logger = logger
	}
}

// WithConfirm enables publisher confirms for reliable delivery.
func WithConfirm(enabled bool) Option {
	return func(p *Publisher) {
		p.useConfirm = enabled
	}
}

// New creates a new Publisher.
func New(b broker.MessageBroker, opts ...Option) *Publisher {
	p := &Publisher{
		broker:     b,
		logger:     slog.Default(),
		useConfirm: true, // Default to using confirms for reliability
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Publish publishes a response result to the broker.
// The response is serialized to JSON with the body LZMA compressed.
// It publishes to a queue named "results.{request_id}" for correlation,
// OR to the specified replyTo queue if provided.
func (p *Publisher) Publish(ctx context.Context, resp *protocol.Response, replyTo string) error {
	if resp == nil {
		return fmt.Errorf("response cannot be nil")
	}

	if resp.RequestID == "" {
		return fmt.Errorf("response must have a request ID")
	}

	// Build the result message with compressed body
	msg, err := p.buildMessage(resp)
	if err != nil {
		return fmt.Errorf("failed to build message: %w", err)
	}

	// Determine the routing key based on request ID or explicit replyTo
	routingKey := "results." + resp.RequestID
	if replyTo != "" {
		routingKey = replyTo
	}

	p.logger.Debug("publishing result",
		"request_id", resp.RequestID,
		"routing_key", routingKey,
		"status_code", resp.StatusCode,
		"has_error", resp.Error != nil,
		"body_size", len(msg),
	)

	// Publish the message
	if err := p.broker.Publish(ctx, "", routingKey, msg); err != nil {
		return fmt.Errorf("failed to publish result: %w", err)
	}

	return nil
}

// ResultMessage is the wire format for result messages.
// The Body field is LZMA compressed to reduce message size.
type ResultMessage struct {
	// RequestID correlates this response to the original request.
	RequestID string `json:"request_id"`

	// EndpointID identifies which endpoint handled this request.
	EndpointID string `json:"endpoint_id,omitempty"`

	// SessionID if a session was created or used.
	SessionID string `json:"session_id,omitempty"`

	// StatusCode is the HTTP status code received from the target.
	StatusCode int `json:"status_code"`

	// Headers contains the HTTP response headers.
	Headers protocol.HeaderMap `json:"headers"`

	// CompressedBody contains the LZMA compressed response body.
	CompressedBody []byte `json:"body,omitempty"`

	// BodyCompressed indicates if the body is LZMA compressed.
	BodyCompressed bool `json:"body_compressed"`

	// Error contains error details if the request failed.
	Error *protocol.ErrorInfo `json:"error,omitempty"`

	// Timing contains request timing details for observability.
	Timing *protocol.TimingInfo `json:"timing,omitempty"`
}

// buildMessage builds a ResultMessage from a protocol.Response.
func (p *Publisher) buildMessage(resp *protocol.Response) ([]byte, error) {
	msg := ResultMessage{
		RequestID:  resp.RequestID,
		EndpointID: resp.EndpointID,
		SessionID:  resp.SessionID,
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Error:      resp.Error,
		Timing:     resp.Timing,
	}

	// Compress the body if present
	if len(resp.Body) > 0 {
		compressed, err := protocol.Compress(resp.Body)
		if err != nil {
			p.logger.Warn("failed to compress body, sending uncompressed",
				"request_id", resp.RequestID,
				"error", err,
				"body_size", len(resp.Body),
			)
			// Fall back to uncompressed
			msg.CompressedBody = resp.Body
			msg.BodyCompressed = false
		} else {
			msg.CompressedBody = compressed
			msg.BodyCompressed = true

			ratio := protocol.CompressionRatio(resp.Body, compressed)
			p.logger.Debug("compressed response body",
				"request_id", resp.RequestID,
				"original_size", len(resp.Body),
				"compressed_size", len(compressed),
				"ratio", fmt.Sprintf("%.2f", ratio),
			)
		}
	}

	return json.Marshal(msg)
}

// PublishError publishes an error result for a request that failed.
// This is a convenience method for creating error responses.
func (p *Publisher) PublishError(ctx context.Context, requestID, endpointID string, errInfo *protocol.ErrorInfo, replyTo string) error {
	resp := &protocol.Response{
		RequestID:  requestID,
		EndpointID: endpointID,
		StatusCode: 0, // No HTTP status for errors
		Error:      errInfo,
	}
	return p.Publish(ctx, resp, replyTo)
}

// NewNetworkError creates an ErrorInfo for network-related failures.
func NewNetworkError(message string, retryable bool) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "network error: " + message,
		Retryable: retryable,
	}
}

// NewTLSError creates an ErrorInfo for TLS-related failures.
func NewTLSError(message string) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "tls error: " + message,
		Retryable: false, // TLS errors are usually not retryable
	}
}

// NewHTTPError creates an ErrorInfo for HTTP-level errors from upstream.
func NewHTTPError(statusCode int, message string) *protocol.ErrorInfo {
	// 5xx errors are typically retryable, 4xx are not
	retryable := statusCode >= 500 && statusCode < 600
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   fmt.Sprintf("http error %d: %s", statusCode, message),
		Retryable: retryable,
	}
}

// NewTimeoutError creates an ErrorInfo for timeout-related failures.
func NewTimeoutError(message string) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeEndpointTimeout,
		Message:   "timeout: " + message,
		Retryable: true,
	}
}

// Handler returns a result handler function suitable for use with the consumer.
// This bridges the publisher to the consumer's resultHandler callback.
func (p *Publisher) Handler() func(ctx context.Context, resp *protocol.Response, replyTo string) error {
	return p.Publish
}

// Package orchestrator provides task orchestration for the Relay Server.
// This includes publishing tasks to endpoints and consuming results.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// ResultMessage is the wire format for result messages from endpoints.
// The Body field is Zstd compressed to reduce message size.
// This matches the format sent by the endpoint's publisher.
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

	// CompressedBody contains the Zstd compressed response body.
	CompressedBody []byte `json:"body,omitempty"`

	// BodyCompressed indicates if the body is compressed.
	BodyCompressed bool `json:"body_compressed"`

	// Error contains error details if the request failed.
	Error *protocol.ErrorInfo `json:"error,omitempty"`

	// Timing contains request timing details for observability.
	Timing *protocol.TimingInfo `json:"timing,omitempty"`
}

// Consumer handles result consumption from endpoints via the message broker.
type Consumer struct {
	broker  broker.MessageBroker
	timeout time.Duration
	logger  *slog.Logger
}

// ConsumerOption is a functional option for configuring the Consumer.
type ConsumerOption func(*Consumer)

// WithTimeout sets the timeout for waiting for results.
func WithTimeout(timeout time.Duration) ConsumerOption {
	return func(c *Consumer) {
		c.timeout = timeout
	}
}

// WithConsumerLogger sets the logger for the consumer.
func WithConsumerLogger(logger *slog.Logger) ConsumerOption {
	return func(c *Consumer) {
		c.logger = logger
	}
}

// DefaultResultTimeout is the default timeout for waiting for results (30 seconds).
const DefaultResultTimeout = 30 * time.Second

// NewConsumer creates a new Consumer.
func NewConsumer(b broker.MessageBroker, opts ...ConsumerOption) *Consumer {
	c := &Consumer{
		broker:  b,
		timeout: DefaultResultTimeout,
		logger:  slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// WaitForResult waits for a result on the given queue with the configured timeout.
// Returns the parsed ResultMessage or an error (including timeout).
func (c *Consumer) WaitForResult(ctx context.Context, resultQueue string) (*ResultMessage, error) {
	c.logger.Debug("waiting for result",
		"queue", resultQueue,
		"timeout", c.timeout,
	)

	// Use the broker's ConsumeOnce to wait for a single message
	body, err := c.broker.ConsumeOnce(ctx, resultQueue, c.timeout)
	if err != nil {
		if errors.Is(err, broker.ErrTimeout) {
			c.logger.Warn("timeout waiting for result",
				"queue", resultQueue,
				"timeout", c.timeout,
			)
			return nil, ErrResultTimeout
		}
		return nil, fmt.Errorf("failed to consume result: %w", err)
	}

	// Parse the result message
	result := AcquireResultMessage()
	if err := json.Unmarshal(body, result); err != nil {
		ReleaseResultMessage(result)
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	c.logger.Debug("received result",
		"queue", resultQueue,
		"request_id", result.RequestID,
		"status_code", result.StatusCode,
		"has_error", result.Error != nil,
	)

	// Decompress body if needed
	if result.BodyCompressed && len(result.CompressedBody) > 0 {
		decompressed, err := protocol.Decompress(result.CompressedBody)
		if err != nil {
			c.logger.Warn("failed to decompress body, using compressed",
				"request_id", result.RequestID,
				"error", err,
			)
			// Fall back to compressed body
			result.BodyCompressed = false
		} else {
			result.CompressedBody = decompressed
			result.BodyCompressed = false
			c.logger.Debug("decompressed response body",
				"request_id", result.RequestID,
				"size", len(decompressed),
			)
		}
	}

	return result, nil
}

// ToResponse converts a ResultMessage to a protocol.Response.
func (r *ResultMessage) ToResponse() *protocol.Response {
	return &protocol.Response{
		RequestID:  r.RequestID,
		EndpointID: r.EndpointID,
		SessionID:  r.SessionID,
		StatusCode: r.StatusCode,
		Headers:    r.Headers,
		Body:       r.CompressedBody, // Already decompressed by WaitForResult
		Error:      r.Error,
		Timing:     r.Timing,
	}
}

// ErrResultTimeout is returned when waiting for a result times out.
var ErrResultTimeout = errors.New("timeout waiting for result from endpoint")

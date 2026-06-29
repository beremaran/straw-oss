// Package orchestrator manages the request-response lifecycle across endpoints.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

// ResultMessage carries the response from an endpoint back to the relay.
type ResultMessage struct {
	RequestID      string               `json:"request_id"`
	EndpointID     string               `json:"endpoint_id,omitempty"`
	SessionID      string               `json:"session_id,omitempty"`
	StatusCode     int                  `json:"status_code"`
	Headers        protocol.HeaderMap   `json:"headers"`
	CompressedBody []byte               `json:"body,omitempty"`
	BodyCompressed bool                 `json:"body_compressed"`
	Error          *protocol.ErrorInfo  `json:"error,omitempty"`
	Timing         *protocol.TimingInfo `json:"timing,omitempty"`
}

// Consumer receives result messages from endpoints via the message broker.
type Consumer struct {
	broker  broker.MessageBroker
	timeout time.Duration
	logger  *slog.Logger
}

// ConsumerOption configures a Consumer.
type ConsumerOption func(*Consumer)

// WithTimeout sets the result wait timeout for the consumer.
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

// DefaultResultTimeout is the default wait duration for endpoint results.
const DefaultResultTimeout = 30 * time.Second

// NewConsumer creates a Consumer with the given broker and options.
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

// WaitForResult blocks until a result arrives on the given subject or timeout expires.
func (c *Consumer) WaitForResult(ctx context.Context, resultSubject string) (*ResultMessage, error) {
	c.logger.Debug("waiting for result",
		"subject", resultSubject,
		"timeout", c.timeout,
	)

	body, err := c.broker.ConsumeOnce(ctx, resultSubject, c.timeout)
	if err != nil {
		if errors.Is(err, broker.ErrTimeout) {
			c.logger.Warn("timeout waiting for result",
				"subject", resultSubject,
				"timeout", c.timeout,
			)

			return nil, ErrResultTimeout
		}

		return nil, fmt.Errorf("failed to consume result: %w", err)
	}

	result := AcquireResultMessage()

	err = json.Unmarshal(body, result)
	if err != nil {
		ReleaseResultMessage(result)

		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	c.logger.Debug("received result",
		"subject", resultSubject,
		"request_id", result.RequestID,
		"status_code", result.StatusCode,
		"has_error", result.Error != nil,
	)

	if result.BodyCompressed && len(result.CompressedBody) > 0 {
		decompressed, err := protocol.Decompress(result.CompressedBody)
		if err != nil {
			c.logger.Warn("failed to decompress body, using compressed",
				"request_id", result.RequestID,
				"error", err,
			)

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

// ToResponse converts the ResultMessage into a protocol.Response.
func (r *ResultMessage) ToResponse() *protocol.Response {
	return &protocol.Response{
		RequestID:  r.RequestID,
		EndpointID: r.EndpointID,
		SessionID:  r.SessionID,
		StatusCode: r.StatusCode,
		Headers:    r.Headers,
		Body:       r.CompressedBody,
		Error:      r.Error,
		Timing:     r.Timing,
	}
}

// ErrResultTimeout is returned when no result arrives within the configured timeout.
var ErrResultTimeout = errors.New("timeout waiting for result from endpoint")

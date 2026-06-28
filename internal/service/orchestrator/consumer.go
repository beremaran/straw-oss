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

type ResultMessage struct {
	RequestID string `json:"request_id"`

	EndpointID string `json:"endpoint_id,omitempty"`

	SessionID string `json:"session_id,omitempty"`

	StatusCode int `json:"status_code"`

	Headers protocol.HeaderMap `json:"headers"`

	CompressedBody []byte `json:"body,omitempty"`

	BodyCompressed bool `json:"body_compressed"`

	Error *protocol.ErrorInfo `json:"error,omitempty"`

	Timing *protocol.TimingInfo `json:"timing,omitempty"`
}

type Consumer struct {
	broker  broker.MessageBroker
	timeout time.Duration
	logger  *slog.Logger
}

type ConsumerOption func(*Consumer)

func WithTimeout(timeout time.Duration) ConsumerOption {
	return func(c *Consumer) {
		c.timeout = timeout
	}
}

func WithConsumerLogger(logger *slog.Logger) ConsumerOption {
	return func(c *Consumer) {
		c.logger = logger
	}
}

const DefaultResultTimeout = 30 * time.Second

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

func (c *Consumer) WaitForResult(ctx context.Context, resultQueue string) (*ResultMessage, error) {
	c.logger.Debug("waiting for result",
		"queue", resultQueue,
		"timeout", c.timeout,
	)

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

var ErrResultTimeout = errors.New("timeout waiting for result from endpoint")

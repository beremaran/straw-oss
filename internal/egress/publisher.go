package egress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/beremaran/straw/internal/protocol"
)

var (
	// ErrResponseCannotBeNil is returned when Publish is called with a nil response.
	ErrResponseCannotBeNil = errors.New("response cannot be nil")
	// ErrMissingRequestID is returned when a response lacks a request ID.
	ErrMissingRequestID = errors.New("response must have a request ID")
)

type publishBroker interface {
	Publish(ctx context.Context, subject string, body []byte) error
}

// Publisher publishes task results to a message broker.
type Publisher struct {
	broker publishBroker
	logger *slog.Logger
}

// NewPublisher creates a new Publisher with the given broker.
func NewPublisher(b publishBroker) *Publisher {
	return &Publisher{
		broker: b,
		logger: slog.Default(),
	}
}

// Publish marshals and sends a task result to the message broker.
func (p *Publisher) Publish(ctx context.Context, resp *protocol.Response, replyTo string) error {
	if resp == nil {
		return ErrResponseCannotBeNil
	}

	if resp.RequestID == "" {
		return ErrMissingRequestID
	}

	msg, err := p.buildMessage(resp)
	if err != nil {
		return fmt.Errorf("failed to build message: %w", err)
	}

	resultSubject := "results." + resp.RequestID
	if replyTo != "" {
		resultSubject = replyTo
	}

	p.logger.Debug("publishing result",
		"request_id", resp.RequestID,
		"subject", resultSubject,
		"status_code", resp.StatusCode,
		"has_error", resp.Error != nil,
		"body_size", len(msg),
	)

	err = p.broker.Publish(ctx, resultSubject, msg)
	if err != nil {
		return fmt.Errorf("failed to publish result: %w", err)
	}

	return nil
}

func (p *Publisher) buildMessage(resp *protocol.Response) ([]byte, error) {
	data, err := protocol.MarshalResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("marshal result message: %w", err)
	}

	return data, nil
}

package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/protocol"
)

const (
	// HTTPServerErrorCode is the lowest HTTP status code for server errors (5xx).
	HTTPServerErrorCode = 500
	// HTTPStatusClassUpperBound is the exclusive upper bound for 5xx status codes.
	HTTPStatusClassUpperBound = 600
)

var (
	// ErrResponseCannotBeNil is returned when Publish is called with a nil response.
	ErrResponseCannotBeNil = errors.New("response cannot be nil")
	// ErrMissingRequestID is returned when a response lacks a request ID.
	ErrMissingRequestID = errors.New("response must have a request ID")
)

// Publisher publishes task results to a message broker.
type Publisher struct {
	broker broker.MessageBroker
	logger *slog.Logger
}

// PublisherOption configures a Publisher.
type PublisherOption func(*Publisher)

// WithPublisherLogger sets the logger used by the Publisher.
func WithPublisherLogger(logger *slog.Logger) PublisherOption {
	return func(p *Publisher) {
		p.logger = logger
	}
}

// NewPublisher creates a new Publisher with the given broker and options.
func NewPublisher(b broker.MessageBroker, opts ...PublisherOption) *Publisher {
	p := &Publisher{
		broker: b,
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
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

// ResultMessage is the JSON structure published for task results.
type ResultMessage struct {
	RequestID      string               `json:"request_id"`
	EndpointID     string               `json:"endpoint_id,omitempty"`
	StatusCode     int                  `json:"status_code"`
	Headers        protocol.HeaderMap   `json:"headers"`
	CompressedBody []byte               `json:"body,omitempty"`
	BodyCompressed bool                 `json:"body_compressed"`
	Error          *protocol.ErrorInfo  `json:"error,omitempty"`
	Timing         *protocol.TimingInfo `json:"timing,omitempty"`
}

// PublishError publishes a result containing an error.
func (p *Publisher) PublishError(ctx context.Context, requestID, endpointID string, errInfo *protocol.ErrorInfo, replyTo string) error {
	resp := &protocol.Response{
		RequestID:  requestID,
		EndpointID: endpointID,
		StatusCode: 0,
		Error:      errInfo,
	}

	return p.Publish(ctx, resp, replyTo)
}

// NewNetworkError creates an ErrorInfo for a network failure.
func NewNetworkError(message string, retryable bool) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "network error: " + message,
		Retryable: retryable,
	}
}

// NewTLSError creates an ErrorInfo for a TLS failure.
func NewTLSError(message string) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "tls error: " + message,
		Retryable: false,
	}
}

// NewHTTPError creates an ErrorInfo for an HTTP status code error.
func NewHTTPError(statusCode int, message string) *protocol.ErrorInfo {
	retryable := statusCode >= HTTPServerErrorCode && statusCode < HTTPStatusClassUpperBound

	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   fmt.Sprintf("http error %d: %s", statusCode, message),
		Retryable: retryable,
	}
}

// NewTimeoutError creates an ErrorInfo for a timeout failure.
func NewTimeoutError(message string) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeEndpointTimeout,
		Message:   "timeout: " + message,
		Retryable: true,
	}
}

// Handler returns a function that publishes a task result, suitable as a broker handler.
func (p *Publisher) Handler() func(ctx context.Context, resp *protocol.Response, replyTo string) error {
	return p.Publish
}

func (p *Publisher) buildMessage(resp *protocol.Response) ([]byte, error) {
	msg := ResultMessage{
		RequestID:  resp.RequestID,
		EndpointID: resp.EndpointID,
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Error:      resp.Error,
		Timing:     resp.Timing,
	}

	if len(resp.Body) > 0 {
		compressed, err := protocol.Compress(resp.Body)
		if err != nil {
			p.logger.Warn("failed to compress body, sending uncompressed",
				"request_id", resp.RequestID,
				"error", err,
				"body_size", len(resp.Body),
			)

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

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal result message: %w", err)
	}

	return data, nil
}

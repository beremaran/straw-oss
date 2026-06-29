package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

var (
	ErrResponseCannotBeNil = errors.New("response cannot be nil")
	ErrMissingRequestID    = errors.New("response must have a request ID")
)

type Publisher struct {
	broker     broker.MessageBroker
	logger     *slog.Logger
	useConfirm bool
}

type PublisherOption func(*Publisher)

func WithPublisherLogger(logger *slog.Logger) PublisherOption {
	return func(p *Publisher) {
		p.logger = logger
	}
}

func WithPublisherConfirm(enabled bool) PublisherOption {
	return func(p *Publisher) {
		p.useConfirm = enabled
	}
}

func NewPublisher(b broker.MessageBroker, opts ...PublisherOption) *Publisher {
	p := &Publisher{
		broker:     b,
		logger:     slog.Default(),
		useConfirm: true,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

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

func (p *Publisher) PublishError(ctx context.Context, requestID, endpointID string, errInfo *protocol.ErrorInfo, replyTo string) error {
	resp := &protocol.Response{
		RequestID:  requestID,
		EndpointID: endpointID,
		StatusCode: 0,
		Error:      errInfo,
	}

	return p.Publish(ctx, resp, replyTo)
}

func NewNetworkError(message string, retryable bool) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "network error: " + message,
		Retryable: retryable,
	}
}

func NewTLSError(message string) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   "tls error: " + message,
		Retryable: false,
	}
}

func NewHTTPError(statusCode int, message string) *protocol.ErrorInfo {
	retryable := statusCode >= 500 && statusCode < 600

	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeUpstreamError,
		Message:   fmt.Sprintf("http error %d: %s", statusCode, message),
		Retryable: retryable,
	}
}

func NewTimeoutError(message string) *protocol.ErrorInfo {
	return &protocol.ErrorInfo{
		Code:      protocol.ErrCodeEndpointTimeout,
		Message:   "timeout: " + message,
		Retryable: true,
	}
}

func (p *Publisher) Handler() func(ctx context.Context, resp *protocol.Response, replyTo string) error {
	return p.Publish
}

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

	return json.Marshal(msg)
}

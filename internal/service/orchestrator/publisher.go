package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
	"github.com/google/uuid"
)

type EndpointSelector interface {
	Select(ctx context.Context, rule *domain.RoutingRule) (string, error)

	SelectWithSession(ctx context.Context, sessionID string) (string, error)
}

type Publisher struct {
	broker     broker.MessageBroker
	selector   EndpointSelector
	hmacSecret []byte
	logger     *slog.Logger
	breaker    *circuitbreaker.CircuitBreaker
}

func NewPublisher(b broker.MessageBroker, s EndpointSelector, secret []byte, breaker *circuitbreaker.CircuitBreaker) *Publisher {
	return &Publisher{
		broker:     b,
		selector:   s,
		hmacSecret: secret,
		logger:     slog.Default(),
		breaker:    breaker,
	}
}

func (p *Publisher) Publish(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	sessionID string,
	targetEndpointID string,
	replyTo string,
) (string, error) {
	endpointID, err := p.selectEndpoint(ctx, rule, sessionID, targetEndpointID)
	if err != nil {
		return "", err
	}

	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	if replyTo != "" {
		req.ReplyTo = replyTo
	}
	signedTask, err := protocol.NewSignedTask(req, p.hmacSecret)
	if err != nil {
		return "", fmt.Errorf("failed to create signed task: %w", err)
	}

	body, err := json.Marshal(signedTask)
	if err != nil {
		return "", fmt.Errorf("failed to marshal signed task: %w", err)
	}

	taskSubject := "tasks." + endpointID + ".tasks"

	p.logger.InfoContext(ctx, "task published",
		"request_id", req.ID,
		"endpoint_id", endpointID,
		"subject", taskSubject,
		"reply_to", replyTo,
	)

	err = p.breaker.Execute(func() error {
		return p.broker.Publish(ctx, taskSubject, body)
	})
	if err != nil {
		return "", fmt.Errorf("failed to publish task (circuit breaker): %w", err)
	}

	return endpointID, nil
}

func (p *Publisher) selectEndpoint(
	ctx context.Context,
	rule *domain.RoutingRule,
	sessionID string,
	targetEndpointID string,
) (string, error) {
	if targetEndpointID != "" {
		return targetEndpointID, nil
	}

	endpointID := p.selectSessionEndpoint(ctx, sessionID)
	if endpointID != "" {
		return endpointID, nil
	}

	endpointID, err := p.selector.Select(ctx, rule)
	if err != nil {
		return "", fmt.Errorf("failed to select endpoint: %w", err)
	}

	return endpointID, nil
}

func (p *Publisher) selectSessionEndpoint(ctx context.Context, sessionID string) string {
	if sessionID == "" {
		return ""
	}

	endpointID, err := p.selector.SelectWithSession(ctx, sessionID)
	if err != nil {
		p.logger.Warn("failed to select endpoint from session", "session_id", sessionID, "error", err)

		return ""
	}

	return endpointID
}

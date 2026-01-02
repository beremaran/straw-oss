package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// EndpointSelector selects an appropriate endpoint for a task.
type EndpointSelector interface {
	// Select chooses an endpoint based on the routing rule.
	Select(ctx context.Context, rule *domain.RoutingRule) (string, error)

	// SelectWithSession chooses an endpoint based on an existing session.
	// It may return an error if the session is invalid or the endpoint is unavailable.
	SelectWithSession(ctx context.Context, sessionID string) (string, error)
}

// Publisher publishes tasks to endpoints via the message broker.
type Publisher struct {
	broker     broker.MessageBroker
	selector   EndpointSelector
	hmacSecret []byte
	logger     *slog.Logger
	breaker    *circuitbreaker.CircuitBreaker
}

// NewPublisher creates a new Publisher.
func NewPublisher(b broker.MessageBroker, s EndpointSelector, secret []byte, breaker *circuitbreaker.CircuitBreaker) *Publisher {
	return &Publisher{
		broker:     b,
		selector:   s,
		hmacSecret: secret,
		logger:     slog.Default(),
		breaker:    breaker,
	}
}

// Publish publishes a task to an endpoint queue.
// It handles endpoint selection, task signing, and result queue declaration.
//
// Arguments:
//   - ctx: Context for the operation
//   - req: The request to be processed
//   - rule: The routing rule that matched this request
//   - sessionID: The session ID (if any)
//   - resultHandler: Function to handle the result (can be nil if fire-and-forget, though unlikely for functionality)
//
// Returns:
//   - endpointID: The ID of the selected endpoint
//   - resultQueue: The name of the temporary result queue
//   - err: Any error encountered
func (p *Publisher) Publish(
	ctx context.Context,
	req *protocol.Request,
	rule *domain.RoutingRule,
	sessionID string,
	targetEndpointID string,
	replyTo string,
) (string, error) {
	// 1. Select Endpoint
	var endpointID string
	var err error

	if targetEndpointID != "" {
		endpointID = targetEndpointID
	} else {
		if sessionID != "" {
			endpointID, err = p.selector.SelectWithSession(ctx, sessionID)
			if err != nil {
				p.logger.Warn("failed to select endpoint from session", "session_id", sessionID, "error", err)
				// Fallback to rule-based selection if session selection fails?
				// For now, depending on requirements, we might want to fail hard or fallback.
				// Assuming session migration logic is handled within SelectWithSession or caller.
			}
		}
	}

	if endpointID == "" {
		endpointID, err = p.selector.Select(ctx, rule)
		if err != nil {
			return "", fmt.Errorf("failed to select endpoint: %w", err)
		}
	}

	// 2. Ensure Request ID
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// 3. Setup Result Queue
	// Handled by caller (replyTo)

	// 4. Create Signed Task
	// Update request with session info if needed (already in req usually)
	// Force the replyTo queue on the request if provided
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

	// 5. Publish to Endpoint Queue
	endpointQueue := "endpoint." + endpointID + ".tasks"

	p.logger.InfoContext(ctx, "task published",
		"request_id", req.ID,
		"endpoint_id", endpointID,
		"queue", endpointQueue,
		"result_queue", replyTo,
	)

	// Wrap publication in circuit breaker
	err = p.breaker.Execute(func() error {
		return p.broker.Publish(ctx, "tasks", endpointQueue, body)
	})
	if err != nil {
		return "", fmt.Errorf("failed to publish task (circuit breaker): %w", err)
	}

	return endpointID, nil
}

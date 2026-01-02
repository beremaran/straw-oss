// Package endpoint provides services for managing endpoint health and state.
package endpoint

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

// HeartbeatMessage is the wire format for heartbeat messages.
// This mirrors the format from internal/endpoint/heartbeat/sender.go.
type HeartbeatMessage struct {
	EndpointID  string   `json:"endpoint_id"`
	Timestamp   int64    `json:"ts"`
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
}

// HealthService manages endpoint health state by consuming heartbeats.
type HealthService struct {
	broker broker.MessageBroker
	store  redis.HealthStore
	logger *slog.Logger
	queue  string

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// HealthOption is a functional option for configuring the HealthService.
type HealthOption func(*HealthService)

// WithQueue sets the queue name for consuming heartbeats.
func WithQueue(queue string) HealthOption {
	return func(s *HealthService) {
		s.queue = queue
	}
}

// WithHealthLogger sets the logger for the health service.
func WithHealthLogger(logger *slog.Logger) HealthOption {
	return func(s *HealthService) {
		s.logger = logger
	}
}

// NewHealthService creates a new HealthService.
func NewHealthService(b broker.MessageBroker, store redis.HealthStore, opts ...HealthOption) *HealthService {
	s := &HealthService{
		broker: b,
		store:  store,
		logger: slog.Default(),
		queue:  "heartbeats",
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start begins consuming heartbeat messages.
// This method returns immediately and runs consumption in a goroutine.
func (s *HealthService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.logger.Warn("health service already running")
		return nil
	}

	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	s.running = true

	s.logger.Info("starting health service", "queue", s.queue)

	// Start subscription in background
	go s.run(ctx)

	return nil
}

// Stop gracefully stops the health service.
func (s *HealthService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	done := s.done
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	s.logger.Info("health service stopped")
}

// IsRunning returns true if the service is currently running.
func (s *HealthService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// run consumes heartbeat messages.
func (s *HealthService) run(ctx context.Context) {
	defer close(s.done)

	err := s.broker.Subscribe(ctx, s.queue, s.handleHeartbeat)
	if err != nil {
		s.logger.Error("failed to subscribe to heartbeats", "error", err)
		return
	}

	// Wait for context cancellation
	<-ctx.Done()
}

// handleHeartbeat processes a single heartbeat message.
func (s *HealthService) handleHeartbeat(ctx context.Context, body []byte) error {
	var msg HeartbeatMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		s.logger.Error("failed to unmarshal heartbeat", "error", err)
		return nil // Don't nack malformed messages, just drop them
	}

	if msg.EndpointID == "" {
		s.logger.Warn("received heartbeat with empty endpoint_id")
		return nil
	}

	// Convert to health record
	health := &redis.EndpointHealth{
		EndpointID:  msg.EndpointID,
		State:       redis.HealthStateHealthy, // Heartbeat received = healthy
		Tags:        msg.Tags,
		Version:     msg.Version,
		ActiveTasks: msg.ActiveTasks,
		LastSeen:    time.Unix(msg.Timestamp, 0),
	}

	// Check if endpoint is draining
	isDraining, err := s.store.IsDraining(ctx, msg.EndpointID)
	if err == nil && isDraining {
		health.State = redis.HealthStateDraining
	}

	// Use current time if heartbeat timestamp is missing or too old
	if msg.Timestamp == 0 || time.Since(health.LastSeen) > time.Hour {
		health.LastSeen = time.Now()
	}

	if err := s.store.UpdateHealth(ctx, health); err != nil {
		s.logger.Error("failed to update endpoint health",
			"endpoint_id", msg.EndpointID,
			"error", err,
		)
		return err // Nack for retry on store errors
	}

	s.logger.Debug("processed heartbeat",
		"endpoint_id", msg.EndpointID,
		"active_tasks", msg.ActiveTasks,
	)

	return nil
}

// GetHealthyEndpoints returns healthy endpoints matching the given tags.
func (s *HealthService) GetHealthyEndpoints(ctx context.Context, tags []string) ([]*redis.EndpointHealth, error) {
	return s.store.ListHealthyByTags(ctx, tags)
}

// IsEndpointHealthy checks if a specific endpoint is healthy.
func (s *HealthService) IsEndpointHealthy(ctx context.Context, endpointID string) (bool, error) {
	health, err := s.store.GetHealth(ctx, endpointID)
	if err != nil {
		return false, err
	}

	return health.State == redis.HealthStateHealthy || health.State == redis.HealthStateSuspect, nil
}

// GetEndpointHealth returns the health state of a specific endpoint.
func (s *HealthService) GetEndpointHealth(ctx context.Context, endpointID string) (*redis.EndpointHealth, error) {
	return s.store.GetHealth(ctx, endpointID)
}

// DrainEndpoint marks an endpoint as draining.
func (s *HealthService) DrainEndpoint(ctx context.Context, endpointID string) error {
	return s.store.SetDraining(ctx, endpointID, true)
}

// ListAllEndpoints returns all known endpoints.
func (s *HealthService) ListAllEndpoints(ctx context.Context) ([]*redis.EndpointHealth, error) {
	return s.store.ListAllEndpoints(ctx)
}

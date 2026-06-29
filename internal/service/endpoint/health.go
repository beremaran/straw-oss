package endpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/pkg/broker"
)

const defaultHeartbeatSubject = "heartbeats.>"

// HeartbeatMessage represents a heartbeat from an endpoint.
type HeartbeatMessage struct {
	EndpointID  string   `json:"endpoint_id"`
	Timestamp   int64    `json:"ts"`
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
}

// HealthService manages endpoint health tracking via heartbeats.
type HealthService struct {
	broker           broker.MessageBroker
	store            redis.HealthStore
	logger           *slog.Logger
	heartbeatSubject string
	mu               sync.Mutex
	running          bool
	cancel           context.CancelFunc
	done             chan struct{}
}

// HealthOption configures a HealthService.
type HealthOption func(*HealthService)

// WithHeartbeatSubject sets the NATS subject for heartbeat messages.
func WithHeartbeatSubject(subject string) HealthOption {
	return func(s *HealthService) {
		s.heartbeatSubject = subject
	}
}

// WithHealthLogger sets the logger for the health service.
func WithHealthLogger(logger *slog.Logger) HealthOption {
	return func(s *HealthService) {
		s.logger = logger
	}
}

// NewHealthService creates a new HealthService with the given broker and store.
func NewHealthService(b broker.MessageBroker, store redis.HealthStore, opts ...HealthOption) *HealthService {
	s := &HealthService{
		broker:           b,
		store:            store,
		logger:           slog.Default(),
		heartbeatSubject: defaultHeartbeatSubject,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start begins processing heartbeats. It is safe to call multiple times.
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

	s.logger.Info("starting health service", "subject", s.heartbeatSubject)

	go s.run(ctx)

	return nil
}

// Stop signals the health service to stop and waits for goroutines to exit.
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

// IsRunning reports whether the health service is currently running.
func (s *HealthService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running
}

// GetHealthyEndpoints returns all endpoints in healthy or suspect state.
func (s *HealthService) GetHealthyEndpoints(ctx context.Context, tags []string) ([]*redis.EndpointHealth, error) {
	endpoints, err := s.store.ListHealthyByTags(ctx, tags)
	if err != nil {
		return nil, fmt.Errorf("list healthy endpoints: %w", err)
	}

	return endpoints, nil
}

// IsEndpointHealthy reports whether the endpoint is healthy or suspect.
func (s *HealthService) IsEndpointHealthy(ctx context.Context, endpointID string) (bool, error) {
	health, err := s.store.GetHealth(ctx, endpointID)
	if err != nil {
		return false, fmt.Errorf("get health for %s: %w", endpointID, err)
	}

	return health.State == redis.HealthStateHealthy || health.State == redis.HealthStateSuspect, nil
}

// GetEndpointHealth returns the health state for a single endpoint.
func (s *HealthService) GetEndpointHealth(ctx context.Context, endpointID string) (*redis.EndpointHealth, error) {
	health, err := s.store.GetHealth(ctx, endpointID)
	if err != nil {
		return nil, fmt.Errorf("get health for %s: %w", endpointID, err)
	}

	return health, nil
}

// DrainEndpoint marks an endpoint as draining.
func (s *HealthService) DrainEndpoint(ctx context.Context, endpointID string) error {
	err := s.store.SetDraining(ctx, endpointID, true)
	if err != nil {
		return fmt.Errorf("drain endpoint %s: %w", endpointID, err)
	}

	return nil
}

// SetDraining marks or unmarks an endpoint as draining.
func (s *HealthService) SetDraining(ctx context.Context, endpointID string, draining bool) error {
	err := s.store.SetDraining(ctx, endpointID, draining)
	if err != nil {
		return fmt.Errorf("set draining for %s: %w", endpointID, err)
	}

	return nil
}

// SetDeleted marks or unmarks an endpoint as deleted.
func (s *HealthService) SetDeleted(ctx context.Context, endpointID string, deleted bool) error {
	err := s.store.SetDeleted(ctx, endpointID, deleted)
	if err != nil {
		return fmt.Errorf("set deleted for %s: %w", endpointID, err)
	}

	return nil
}

// DeleteHealth removes health data for an endpoint.
func (s *HealthService) DeleteHealth(ctx context.Context, endpointID string) error {
	err := s.store.DeleteHealth(ctx, endpointID)
	if err != nil {
		return fmt.Errorf("delete health for %s: %w", endpointID, err)
	}

	return nil
}

// ListAllEndpoints returns all tracked endpoints.
func (s *HealthService) ListAllEndpoints(ctx context.Context) ([]*redis.EndpointHealth, error) {
	endpoints, err := s.store.ListAllEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all endpoints: %w", err)
	}

	return endpoints, nil
}

func (s *HealthService) run(ctx context.Context) {
	defer close(s.done)

	err := s.broker.Subscribe(ctx, s.heartbeatSubject, s.handleHeartbeat, broker.WithTransient())
	if err != nil {
		s.logger.Error("failed to subscribe to heartbeats", "error", err)

		return
	}

	<-ctx.Done()
}

func (s *HealthService) handleHeartbeat(ctx context.Context, body []byte) error {
	var msg HeartbeatMessage

	err := json.Unmarshal(body, &msg)
	if err != nil {
		s.logger.Error("failed to unmarshal heartbeat", "error", err)

		return nil
	}

	if msg.EndpointID == "" {
		s.logger.Warn("received heartbeat with empty endpoint_id")

		return nil
	}

	health := &redis.EndpointHealth{
		EndpointID:  msg.EndpointID,
		State:       redis.HealthStateHealthy,
		Tags:        msg.Tags,
		Version:     msg.Version,
		ActiveTasks: msg.ActiveTasks,
		LastSeen:    time.Unix(msg.Timestamp, 0),
	}

	isDraining, err := s.store.IsDraining(ctx, msg.EndpointID)
	if err == nil && isDraining {
		health.State = redis.HealthStateDraining
	}

	isDeleted, err := s.store.IsDeleted(ctx, msg.EndpointID)
	if err == nil && isDeleted {
		health.State = redis.HealthStateDeleted
	}

	if msg.Timestamp == 0 || time.Since(health.LastSeen) > time.Hour {
		health.LastSeen = time.Now()
	}

	err = s.store.UpdateHealth(ctx, health)
	if err != nil {
		s.logger.Error("failed to update endpoint health",
			"endpoint_id", msg.EndpointID,
			"error", err,
		)

		return fmt.Errorf("update health for %s: %w", msg.EndpointID, err)
	}

	s.logger.Debug("processed heartbeat",
		"endpoint_id", msg.EndpointID,
		"active_tasks", msg.ActiveTasks,
	)

	return nil
}

//nolint:funcorder
package endpoint

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/pkg/broker"
)

type HeartbeatMessage struct {
	EndpointID  string   `json:"endpoint_id"`
	Timestamp   int64    `json:"ts"`
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
}

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

type HealthOption func(*HealthService)

func WithQueue(queue string) HealthOption {
	return func(s *HealthService) {
		s.queue = queue
	}
}

func WithHealthLogger(logger *slog.Logger) HealthOption {
	return func(s *HealthService) {
		s.logger = logger
	}
}

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

	go s.run(ctx)

	return nil
}

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

func (s *HealthService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running
}

func (s *HealthService) run(ctx context.Context) {
	defer close(s.done)

	err := s.broker.Subscribe(ctx, s.queue, s.handleHeartbeat)
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

	if msg.Timestamp == 0 || time.Since(health.LastSeen) > time.Hour {
		health.LastSeen = time.Now()
	}

	err = s.store.UpdateHealth(ctx, health)
	if err != nil {
		s.logger.Error("failed to update endpoint health",
			"endpoint_id", msg.EndpointID,
			"error", err,
		)

		return err
	}

	s.logger.Debug("processed heartbeat",
		"endpoint_id", msg.EndpointID,
		"active_tasks", msg.ActiveTasks,
	)

	return nil
}

func (s *HealthService) GetHealthyEndpoints(ctx context.Context, tags []string) ([]*redis.EndpointHealth, error) {
	return s.store.ListHealthyByTags(ctx, tags)
}

func (s *HealthService) IsEndpointHealthy(ctx context.Context, endpointID string) (bool, error) {
	health, err := s.store.GetHealth(ctx, endpointID)
	if err != nil {
		return false, err
	}

	return health.State == redis.HealthStateHealthy || health.State == redis.HealthStateSuspect, nil
}

func (s *HealthService) GetEndpointHealth(ctx context.Context, endpointID string) (*redis.EndpointHealth, error) {
	return s.store.GetHealth(ctx, endpointID)
}

func (s *HealthService) DrainEndpoint(ctx context.Context, endpointID string) error {
	return s.store.SetDraining(ctx, endpointID, true)
}

func (s *HealthService) ListAllEndpoints(ctx context.Context) ([]*redis.EndpointHealth, error) {
	return s.store.ListAllEndpoints(ctx)
}

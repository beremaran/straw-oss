package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/endpoint/metrics"
)

const DefaultInterval = domain.DefaultHeartbeatInterval

type Message struct {
	EndpointID string `json:"endpoint_id"`

	Timestamp int64 `json:"ts"`

	Version string `json:"version,omitempty"`

	Tags []string `json:"tags,omitempty"`

	ActiveTasks int `json:"active_tasks"`
}

type ActiveTasksFunc func() int

type Sender struct {
	broker     broker.MessageBroker
	endpointID string
	version    string
	tags       []string
	interval   time.Duration
	logger     *slog.Logger

	activeTasksFunc ActiveTasksFunc

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

type Option func(*Sender)

func WithInterval(d time.Duration) Option {
	return func(s *Sender) {
		if d > 0 {
			s.interval = d
		}
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(s *Sender) {
		s.logger = logger
	}
}

func WithVersion(version string) Option {
	return func(s *Sender) {
		s.version = version
	}
}

func WithTags(tags []string) Option {
	return func(s *Sender) {
		s.tags = tags
	}
}

func WithActiveTasksFunc(f ActiveTasksFunc) Option {
	return func(s *Sender) {
		s.activeTasksFunc = f
	}
}

func New(b broker.MessageBroker, endpointID string, opts ...Option) *Sender {
	s := &Sender{
		broker:          b,
		endpointID:      endpointID,
		interval:        DefaultInterval,
		logger:          slog.Default(),
		activeTasksFunc: func() int { return 0 },
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Sender) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.logger.Warn("heartbeat sender already running", "endpoint_id", s.endpointID)
		return
	}

	ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	s.running = true

	s.logger.Info("starting heartbeat sender",
		"endpoint_id", s.endpointID,
		"interval", s.interval,
	)

	go s.run(ctx)
}

func (s *Sender) Stop() {
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

	s.logger.Info("heartbeat sender stopped", "endpoint_id", s.endpointID)
}

func (s *Sender) run(ctx context.Context) {
	defer close(s.done)

	s.sendHeartbeat(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendHeartbeat(ctx)
		}
	}
}

func (s *Sender) sendHeartbeat(ctx context.Context) {
	msg := Message{
		EndpointID:  s.endpointID,
		Timestamp:   time.Now().Unix(),
		Version:     s.version,
		Tags:        s.tags,
		ActiveTasks: s.activeTasksFunc(),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		s.logger.Error("failed to marshal heartbeat",
			"endpoint_id", s.endpointID,
			"error", err,
		)
		return
	}

	if err := s.broker.Publish(ctx, "heartbeats", s.endpointID, data); err != nil {
		s.logger.Error("failed to publish heartbeat",
			"endpoint_id", s.endpointID,
			"error", err,
		)
		return
	}

	metrics.HeartbeatsSent.Inc()

	s.logger.Debug("heartbeat sent",
		"endpoint_id", s.endpointID,
		"active_tasks", msg.ActiveTasks,
	)
}

func (s *Sender) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

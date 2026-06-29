package endpoint

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/endpoint/metrics"
	"github.com/beremaran/straw/pkg/broker"
)

// DefaultHeartbeatInterval is the default frequency for heartbeat messages.
const DefaultHeartbeatInterval = 10 * time.Second

// HeartbeatMessage is the JSON structure published for heartbeats.
type HeartbeatMessage struct {
	EndpointID  string   `json:"endpoint_id"`
	Timestamp   int64    `json:"ts"`
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ActiveTasks int      `json:"active_tasks"`
}

// ActiveTasksFunc returns the current number of active tasks.
type ActiveTasksFunc func() int

// HeartbeatSender periodically publishes heartbeat messages to a message broker.
type HeartbeatSender struct {
	broker          broker.MessageBroker
	endpointID      string
	version         string
	tags            []string
	interval        time.Duration
	logger          *slog.Logger
	activeTasksFunc ActiveTasksFunc
	mu              sync.Mutex
	running         bool
	cancel          context.CancelFunc
	done            chan struct{}
}

// HeartbeatOption configures a HeartbeatSender.
type HeartbeatOption func(*HeartbeatSender)

// WithHeartbeatInterval sets the interval between heartbeat messages.
func WithHeartbeatInterval(d time.Duration) HeartbeatOption {
	return func(s *HeartbeatSender) {
		if d > 0 {
			s.interval = d
		}
	}
}

// WithHeartbeatLogger sets the logger used by the HeartbeatSender.
func WithHeartbeatLogger(logger *slog.Logger) HeartbeatOption {
	return func(s *HeartbeatSender) {
		s.logger = logger
	}
}

// WithHeartbeatVersion sets the version string included in heartbeat messages.
func WithHeartbeatVersion(version string) HeartbeatOption {
	return func(s *HeartbeatSender) {
		s.version = version
	}
}

// WithHeartbeatTags sets the tags included in heartbeat messages.
func WithHeartbeatTags(tags []string) HeartbeatOption {
	return func(s *HeartbeatSender) {
		s.tags = tags
	}
}

// WithHeartbeatActiveTasksFunc sets a callback for the active task count.
func WithHeartbeatActiveTasksFunc(f ActiveTasksFunc) HeartbeatOption {
	return func(s *HeartbeatSender) {
		s.activeTasksFunc = f
	}
}

// NewHeartbeatSender creates a new HeartbeatSender with the given broker and options.
func NewHeartbeatSender(b broker.MessageBroker, endpointID string, opts ...HeartbeatOption) *HeartbeatSender {
	s := &HeartbeatSender{
		broker:          b,
		endpointID:      endpointID,
		interval:        DefaultHeartbeatInterval,
		logger:          slog.Default(),
		activeTasksFunc: func() int { return 0 },
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start begins periodic heartbeat sending and returns immediately.
func (s *HeartbeatSender) Start(ctx context.Context) {
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

// Stop halts the heartbeat sender and waits for the goroutine to exit.
func (s *HeartbeatSender) Stop() {
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

// IsRunning reports whether the heartbeat sender is currently active.
func (s *HeartbeatSender) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.running
}

func (s *HeartbeatSender) run(ctx context.Context) {
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

func (s *HeartbeatSender) sendHeartbeat(ctx context.Context) {
	msg := HeartbeatMessage{
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

	err = s.broker.Publish(ctx, "heartbeats."+s.endpointID, data)
	if err != nil {
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

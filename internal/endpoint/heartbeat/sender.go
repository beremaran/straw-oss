// Package heartbeat provides periodic heartbeat publishing for the Endpoint worker.
// Heartbeats indicate endpoint health to the Relay Server.
package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/metrics"
)

// DefaultInterval is the default interval between heartbeats.
// Per design doc Section 7.3, endpoints send heartbeats every 10 seconds.
const DefaultInterval = domain.DefaultHeartbeatInterval

// HeartbeatMessage is the wire format for heartbeat messages.
type HeartbeatMessage struct {
	// EndpointID is the unique identifier for this endpoint.
	EndpointID string `json:"endpoint_id"`

	// Timestamp is the Unix timestamp when this heartbeat was sent.
	Timestamp int64 `json:"ts"`

	// Version is the endpoint software version.
	Version string `json:"version,omitempty"`

	// Tags describe the endpoint's capabilities and properties.
	Tags []string `json:"tags,omitempty"`

	// ActiveTasks is the number of currently processing tasks.
	ActiveTasks int `json:"active_tasks"`
}

// ActiveTasksFunc is a callback to get the current number of active tasks.
type ActiveTasksFunc func() int

// Sender sends periodic heartbeats to the message broker.
type Sender struct {
	broker     broker.MessageBroker
	endpointID string
	version    string
	tags       []string
	interval   time.Duration
	logger     *slog.Logger

	// activeTasksFunc returns the current number of active tasks.
	activeTasksFunc ActiveTasksFunc

	// Lifecycle management
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// Option is a functional option for configuring the Sender.
type Option func(*Sender)

// WithInterval sets the heartbeat interval.
func WithInterval(d time.Duration) Option {
	return func(s *Sender) {
		if d > 0 {
			s.interval = d
		}
	}
}

// WithLogger sets the logger for the sender.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Sender) {
		s.logger = logger
	}
}

// WithVersion sets the endpoint version to include in heartbeats.
func WithVersion(version string) Option {
	return func(s *Sender) {
		s.version = version
	}
}

// WithTags sets the endpoint tags to include in heartbeats.
func WithTags(tags []string) Option {
	return func(s *Sender) {
		s.tags = tags
	}
}

// WithActiveTasksFunc sets the callback to get the current active task count.
func WithActiveTasksFunc(f ActiveTasksFunc) Option {
	return func(s *Sender) {
		s.activeTasksFunc = f
	}
}

// New creates a new heartbeat Sender.
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

// Start begins sending periodic heartbeats.
// This method returns immediately and runs heartbeat sending in a goroutine.
// Call Stop() to stop sending heartbeats.
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

// Stop gracefully stops the heartbeat sender.
// It blocks until the sender has fully stopped.
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

// run is the main heartbeat loop.
func (s *Sender) run(ctx context.Context) {
	defer close(s.done)

	// Send initial heartbeat immediately
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

// sendHeartbeat sends a single heartbeat message.
func (s *Sender) sendHeartbeat(ctx context.Context) {
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

	// Publish to "heartbeats" exchange with endpoint ID as routing key
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

// IsRunning returns true if the sender is currently running.
func (s *Sender) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

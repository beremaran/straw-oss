package endpoint

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

// Default thresholds for staleness checking
const (
	// DefaultCheckInterval is how often to check for stale endpoints.
	DefaultCheckInterval = 5 * time.Second

	// DefaultSuspectThreshold is when an endpoint becomes suspect (missed 1 heartbeat).
	DefaultSuspectThreshold = 15 * time.Second

	// DefaultUnhealthyThreshold is when an endpoint becomes unhealthy.
	DefaultUnhealthyThreshold = domain.DefaultHealthThreshold // 30s
)

// StalenessChecker periodically checks for stale endpoints and transitions
// them through the health state machine.
type StalenessChecker struct {
	store              redis.HealthStore
	logger             *slog.Logger
	interval           time.Duration
	suspectThreshold   time.Duration
	unhealthyThreshold time.Duration

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// StalenessOption is a functional option for configuring the StalenessChecker.
type StalenessOption func(*StalenessChecker)

// WithCheckInterval sets the interval between staleness checks.
func WithCheckInterval(d time.Duration) StalenessOption {
	return func(c *StalenessChecker) {
		if d > 0 {
			c.interval = d
		}
	}
}

// WithSuspectThreshold sets the threshold for marking an endpoint as suspect.
func WithSuspectThreshold(d time.Duration) StalenessOption {
	return func(c *StalenessChecker) {
		if d > 0 {
			c.suspectThreshold = d
		}
	}
}

// WithUnhealthyThreshold sets the threshold for marking an endpoint as unhealthy.
func WithUnhealthyThreshold(d time.Duration) StalenessOption {
	return func(c *StalenessChecker) {
		if d > 0 {
			c.unhealthyThreshold = d
		}
	}
}

// WithStalenessLogger sets the logger for the staleness checker.
func WithStalenessLogger(logger *slog.Logger) StalenessOption {
	return func(c *StalenessChecker) {
		c.logger = logger
	}
}

// NewStalenessChecker creates a new StalenessChecker.
func NewStalenessChecker(store redis.HealthStore, opts ...StalenessOption) *StalenessChecker {
	c := &StalenessChecker{
		store:              store,
		logger:             slog.Default(),
		interval:           DefaultCheckInterval,
		suspectThreshold:   DefaultSuspectThreshold,
		unhealthyThreshold: DefaultUnhealthyThreshold,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Start begins periodically checking for stale endpoints.
func (c *StalenessChecker) Start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		c.logger.Warn("staleness checker already running")
		return
	}

	ctx, c.cancel = context.WithCancel(ctx)
	c.done = make(chan struct{})
	c.running = true

	c.logger.Info("starting staleness checker",
		"interval", c.interval,
		"suspect_threshold", c.suspectThreshold,
		"unhealthy_threshold", c.unhealthyThreshold,
	)

	go c.run(ctx)
}

// Stop gracefully stops the staleness checker.
func (c *StalenessChecker) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	cancel := c.cancel
	done := c.done
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	c.logger.Info("staleness checker stopped")
}

// IsRunning returns true if the checker is currently running.
func (c *StalenessChecker) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// run is the main loop that periodically checks for stale endpoints.
func (c *StalenessChecker) run(ctx context.Context) {
	defer close(c.done)

	// Initial check immediately
	c.checkStaleness(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.checkStaleness(ctx)
		}
	}
}

// checkStaleness checks all endpoints for staleness and updates their state.
func (c *StalenessChecker) checkStaleness(ctx context.Context) {
	endpoints, err := c.store.ListAllEndpoints(ctx)
	if err != nil {
		c.logger.Error("failed to list endpoints for staleness check", "error", err)
		return
	}

	now := time.Now()
	for _, ep := range endpoints {
		staleDuration := now.Sub(ep.LastSeen)
		newState := c.determineState(staleDuration, ep.State)

		if newState != ep.State {
			c.logger.Info("endpoint state transition",
				"endpoint_id", ep.EndpointID,
				"old_state", ep.State,
				"new_state", newState,
				"stale_duration", staleDuration,
			)

			ep.State = newState
			if err := c.store.UpdateHealth(ctx, ep); err != nil {
				c.logger.Error("failed to update endpoint state",
					"endpoint_id", ep.EndpointID,
					"error", err,
				)
			}
		}
	}
}

// determineState determines the new state based on staleness duration.
func (c *StalenessChecker) determineState(staleDuration time.Duration, currentState string) string {
	switch {
	case staleDuration >= c.unhealthyThreshold:
		return redis.HealthStateUnhealthy
	case staleDuration >= c.suspectThreshold:
		// Only transition from healthy to suspect, not from unhealthy
		if currentState == redis.HealthStateHealthy {
			return redis.HealthStateSuspect
		}
		// Keep current state if already suspect or unhealthy
		if currentState == redis.HealthStateUnhealthy {
			return redis.HealthStateUnhealthy
		}
		return redis.HealthStateSuspect
	default:
		return redis.HealthStateHealthy
	}
}

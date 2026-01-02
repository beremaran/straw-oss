package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// State represents the current state of the CircuitBreaker.
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// ErrCircuitOpen is returned when the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Config holds the configuration for the CircuitBreaker.
type Config struct {
	// Name is the name of the circuit breaker (useful for logging/monitoring).
	Name string
	// FailureThreshold is the number of failures allowed before opening the circuit.
	FailureThreshold uint
	// ResetTimeout is the duration to wait before switching from Open to Half-Open.
	ResetTimeout time.Duration
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	name             string
	failureThreshold uint
	resetTimeout     time.Duration

	mu          sync.Mutex
	state       State
	failures    uint
	lastFailure time.Time
}

// New creates a new CircuitBreaker with the given configuration.
func New(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 5 // Default
	}
	if cfg.ResetTimeout == 0 {
		cfg.ResetTimeout = 60 * time.Second // Default
	}

	return &CircuitBreaker{
		name:             cfg.Name,
		failureThreshold: cfg.FailureThreshold,
		resetTimeout:     cfg.ResetTimeout,
		state:            StateClosed,
	}
}

// Execute runs the given function if the circuit is closed or half-open.
// It handles state transitions based on the function's error return.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.Allow() {
		return ErrCircuitOpen
	}

	err := fn()
	if err != nil {
		cb.ReportFailure()
		return err
	}

	cb.ReportSuccess()
	return nil
}

// Allow checks if a request can be executed.
// If the state is Open, it checks if the reset timeout has passed to switch to Half-Open.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		// In Half-Open, strictly one request at a time is usually allowed by simple logic,
		// or we trust the caller to be serial or allow concurrent implementation details.
		// For a simple implementation, we allow it, but if it fails it will go back to Open immediately.
		// A more complex implementation might limit to 1 concurrent request.
		// For now, we return true. The first failure will close it again.
		return true
	}
	return false
}

// ReportSuccess reports a successful execution.
func (cb *CircuitBreaker) ReportSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
	} else if cb.state == StateClosed {
		// Optional: reset failures on success in Closed state?
		// Some implementations do, some use a rolling window.
		// A simple counter based approach resets on success.
		cb.failures = 0
	}
}

// ReportFailure reports a failed execution.
func (cb *CircuitBreaker) ReportFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateClosed {
		cb.failures++
		if cb.failures >= cb.failureThreshold {
			cb.state = StateOpen
			cb.lastFailure = time.Now()
		}
	} else if cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.lastFailure = time.Now()
	} else if cb.state == StateOpen {
		// Update last failure time to prolong open state?
		// Usually we reset the timer.
		cb.lastFailure = time.Now()
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

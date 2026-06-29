package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// State represents the current state of the circuit breaker.
type State int

const (
	// StateClosed indicates the circuit is closed and operations are allowed.
	StateClosed State = iota
	// StateOpen indicates the circuit is open and operations are rejected.
	StateOpen
	// StateHalfOpen indicates the circuit is half-open and operations are allowed.
	StateHalfOpen
)

// ErrCircuitOpen is returned when an operation is attempted while the circuit breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

const defaultResetTimeout = 60 * time.Second

// Config holds the configuration for a CircuitBreaker.
type Config struct {
	Name             string
	FailureThreshold uint
	ResetTimeout     time.Duration
}

// CircuitBreaker tracks failures and rejects operations when the failure threshold is exceeded.
type CircuitBreaker struct {
	name             string
	failureThreshold uint
	resetTimeout     time.Duration
	mu               sync.RWMutex
	state            State
	failures         uint
	lastFailure      time.Time
}

// New creates a CircuitBreaker with the given configuration, applying defaults for zero values.
func New(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 5
	}

	if cfg.ResetTimeout == 0 {
		cfg.ResetTimeout = defaultResetTimeout
	}

	return &CircuitBreaker{
		name:             cfg.Name,
		failureThreshold: cfg.FailureThreshold,
		resetTimeout:     cfg.ResetTimeout,
		state:            StateClosed,
	}
}

// Execute runs the provided function, reporting success or failure to the circuit breaker.
// Returns ErrCircuitOpen if the circuit is open.
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

// Allow reports whether an operation should be permitted.
// It returns true when the circuit is closed, half-open, or has transitioned to half-open after the reset timeout.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.RLock()
	state := cb.state
	lastFailure := cb.lastFailure
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateHalfOpen:
		return true
	case StateOpen:
		if time.Since(lastFailure) > cb.resetTimeout {
			cb.mu.Lock()
			defer cb.mu.Unlock()

			if cb.state == StateOpen {
				if time.Since(cb.lastFailure) > cb.resetTimeout {
					cb.state = StateHalfOpen

					return true
				}

				return false
			}

			return true
		}

		return false
	}

	return false
}

// ReportSuccess records a successful operation, resetting the failure count and transitioning from half-open to closed.
func (cb *CircuitBreaker) ReportSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		cb.state = StateClosed
		cb.failures = 0
	case StateClosed:
		cb.failures = 0
	case StateOpen:
		// Do nothing on success in StateOpen (unexpected)
	}
}

// ReportFailure records a failed operation, incrementing the failure count and potentially opening the circuit.
func (cb *CircuitBreaker) ReportFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.failureThreshold {
			cb.state = StateOpen
			cb.lastFailure = time.Now()
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.lastFailure = time.Now()
	case StateOpen:
		cb.lastFailure = time.Now()
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.state
}

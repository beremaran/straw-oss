package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type Config struct {
	Name string

	FailureThreshold uint

	ResetTimeout time.Duration
}

type CircuitBreaker struct {
	name             string
	failureThreshold uint
	resetTimeout     time.Duration

	mu          sync.RWMutex
	state       State
	failures    uint
	lastFailure time.Time
}

func New(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.ResetTimeout == 0 {
		cfg.ResetTimeout = 60 * time.Second
	}

	return &CircuitBreaker{
		name:             cfg.Name,
		failureThreshold: cfg.FailureThreshold,
		resetTimeout:     cfg.ResetTimeout,
		state:            StateClosed,
	}
}

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

func (cb *CircuitBreaker) ReportSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
		cb.failures = 0
	} else if cb.state == StateClosed {

		cb.failures = 0
	}
}

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

		cb.lastFailure = time.Now()
	}
}

func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

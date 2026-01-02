package circuitbreaker

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cfg := Config{
		Name:             "test-breaker",
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
	}
	cb := New(cfg)

	// Initial State: Closed
	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.Allow())

	// 1st Failure
	cb.ReportFailure()
	assert.Equal(t, StateClosed, cb.State())

	// 2nd Failure (Threshold reached)
	cb.ReportFailure()
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow())

	// Try Execute while Open
	err := cb.Execute(func() error { return nil })
	assert.Equal(t, ErrCircuitOpen, err)

	// Wait for ResetTimeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to Half-Open on next Allow check
	assert.True(t, cb.Allow())
	// Note: Allow() changes state to Half-Open inside the lock if conditions met,
	// checking State() immediately after Allow() is safe if no other goroutines.
	assert.Equal(t, StateHalfOpen, cb.State())

	// Success in Half-Open -> Closed
	cb.ReportSuccess()
	assert.Equal(t, StateClosed, cb.State())
	assert.Equal(t, uint(0), cb.failures)
}

func TestCircuitBreaker_HalfOpenToOpen(t *testing.T) {
	cfg := Config{
		Name:             "test-breaker",
		FailureThreshold: 2,
		ResetTimeout:     100 * time.Millisecond,
	}
	cb := New(cfg)

	// Force Open
	cb.ReportFailure()
	cb.ReportFailure()
	assert.Equal(t, StateOpen, cb.State())

	// Wait
	time.Sleep(150 * time.Millisecond)

	// allow -> Half-Open
	assert.True(t, cb.Allow())
	assert.Equal(t, StateHalfOpen, cb.State())

	// Failure in Half-Open -> Open
	cb.ReportFailure()
	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_Execute(t *testing.T) {
	cb := New(Config{FailureThreshold: 1, ResetTimeout: time.Second})

	// Success
	err := cb.Execute(func() error { return nil })
	assert.NoError(t, err)

	// Failure
	failErr := errors.New("fail")
	err = cb.Execute(func() error { return failErr })
	assert.Equal(t, failErr, err)
	assert.Equal(t, StateOpen, cb.State())

	// Blocked
	err = cb.Execute(func() error { return nil })
	assert.Equal(t, ErrCircuitOpen, err)
}

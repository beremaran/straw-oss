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

	assert.Equal(t, StateClosed, cb.State())
	assert.True(t, cb.Allow())

	cb.ReportFailure()
	assert.Equal(t, StateClosed, cb.State())

	cb.ReportFailure()
	assert.Equal(t, StateOpen, cb.State())
	assert.False(t, cb.Allow())

	err := cb.Execute(func() error { return nil })
	assert.Equal(t, ErrCircuitOpen, err)

	time.Sleep(150 * time.Millisecond)

	assert.True(t, cb.Allow())

	assert.Equal(t, StateHalfOpen, cb.State())

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

	cb.ReportFailure()
	cb.ReportFailure()
	assert.Equal(t, StateOpen, cb.State())

	time.Sleep(150 * time.Millisecond)

	assert.True(t, cb.Allow())
	assert.Equal(t, StateHalfOpen, cb.State())

	cb.ReportFailure()
	assert.Equal(t, StateOpen, cb.State())
}

func TestCircuitBreaker_Execute(t *testing.T) {
	cb := New(Config{FailureThreshold: 1, ResetTimeout: time.Second})

	err := cb.Execute(func() error { return nil })
	assert.NoError(t, err)

	failErr := errors.New("fail")
	err = cb.Execute(func() error { return failErr })
	assert.Equal(t, failErr, err)
	assert.Equal(t, StateOpen, cb.State())

	err = cb.Execute(func() error { return nil })
	assert.Equal(t, ErrCircuitOpen, err)
}

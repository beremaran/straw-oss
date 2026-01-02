package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	Init()

	// Verify that metrics are initialized
	assert.NotNil(t, RequestsTotal)
	assert.NotNil(t, RequestDuration)
	assert.NotNil(t, QueueDepth)
	assert.NotNil(t, CacheHits)
	assert.NotNil(t, CacheMisses)
	assert.NotNil(t, ActiveSessions)
	assert.NotNil(t, RateLimitExceeded)

	// Since we are using the global registry, we can't easily check for registration conflicts
	// without resetting the registry, but Init handles idempotency via sync.Once (in the underlying registry init, although our Init here is not protected by Once for the vars).
	// Ideally Init should be safe to call multiple times or the vars check handles it.
	// Our Init implementation re-creates metrics vars but registers them.
	// If called twice, it might panic on duplicate registration?
	// The registry.go implementation uses sync.Once for the registry itself.
	// The variable assignment doesn't matter if registration fails.
}

func TestMetricsRecording(t *testing.T) {
	Init()

	// Test incrementing a counter
	RequestsTotal.WithLabelValues("200", "rule-1", "fp-1").Inc()

	// We can't easily inspect the value without gathering, which is involved.
	// But asserting no panic is a good first step.
	assert.NotPanics(t, func() {
		RequestsTotal.WithLabelValues("200", "rule-1", "fp-1").Inc()
	})

	assert.NotPanics(t, func() {
		RequestDuration.WithLabelValues("rule-1", "200").Observe(0.5)
	})

	assert.NotPanics(t, func() {
		ActiveSessions.Inc()
		ActiveSessions.Dec()
	})
}

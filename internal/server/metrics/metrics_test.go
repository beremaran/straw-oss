package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInit(t *testing.T) {
	Init()

	assert.NotNil(t, RequestsTotal)
	assert.NotNil(t, RequestDuration)
	assert.NotNil(t, QueueDepth)
	assert.NotNil(t, CacheHits)
	assert.NotNil(t, CacheMisses)
	assert.NotNil(t, ActiveSessions)
	assert.NotNil(t, RateLimitExceeded)
}

func TestMetricsRecording(t *testing.T) {
	Init()

	RequestsTotal.WithLabelValues("200", "rule-1", "fp-1").Inc()

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

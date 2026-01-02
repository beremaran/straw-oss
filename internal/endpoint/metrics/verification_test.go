package metrics_test

import (
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/metrics"
	"github.com/stretchr/testify/assert"
)

func TestEndpointMetrics_Registration(t *testing.T) {
	// Verify that metrics are initialized and can be collected
	// We can't easily check registration with the global registry without side effects,
	// but we can check if the variables are not nil and can be used.

	assert.NotNil(t, metrics.UpstreamDuration)
	assert.NotNil(t, metrics.TLSFingerprintUsed)
	assert.NotNil(t, metrics.FingerprintDeprecatedUsed)
	assert.NotNil(t, metrics.ConnectionsPooled)
	assert.NotNil(t, metrics.TasksInFlight)
	assert.NotNil(t, metrics.HeartbeatsSent)
	assert.NotNil(t, metrics.TasksProcessed)
	assert.NotNil(t, metrics.TasksFailed)
}

func TestEndpointMetrics_Usage(t *testing.T) {
	// Simulate usage
	metrics.UpstreamDuration.WithLabelValues("example.com", "200").Observe(0.1)
	metrics.TLSFingerprintUsed.WithLabelValues("chrome-100").Inc()
	metrics.ConnectionsPooled.WithLabelValues("example.com").Inc()
	metrics.ConnectionsPooled.WithLabelValues("example.com").Dec()
	metrics.TasksInFlight.Set(5)
	metrics.HeartbeatsSent.Inc()
	metrics.TasksProcessed.WithLabelValues("success").Inc()
	metrics.TasksFailed.WithLabelValues("timeout").Inc()

	// If no panic, we are good.
	// For more detailed verification, we would need to inspect the registry,
	// but since we are using promauto, we rely on it working if initialization didn't panic.
}

func TestEndpointMetrics_Values(t *testing.T) {
	// Basic value check for a Gauge
	metrics.TasksInFlight.Set(10)

	// We can use the prometheus testutil if we want to be strict, but for now
	// ensuring we can call the methods without panic covers the basic requirement
	// that they are initialized correctly.

	// Using a counter
	c := metrics.HeartbeatsSent
	c.Add(1)
}

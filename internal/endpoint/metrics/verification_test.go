package metrics_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/beremaran/straw/internal/endpoint/metrics"
)

func TestEndpointMetrics_Registration(t *testing.T) {
	assert.NotNil(t, metrics.UpstreamDuration)
	assert.NotNil(t, metrics.TLSFingerprintUsed)
	assert.NotNil(t, metrics.FingerprintDeprecatedUsed)
	assert.NotNil(t, metrics.ConnectionsPooled)
	assert.NotNil(t, metrics.TasksInFlight)
	assert.NotNil(t, metrics.HeartbeatsSent)
	assert.NotNil(t, metrics.TasksProcessed)
	assert.NotNil(t, metrics.TasksFailed)
}

func TestEndpointMetrics_Usage(*testing.T) {
	metrics.UpstreamDuration.WithLabelValues("example.com", "200").Observe(0.1)
	metrics.TLSFingerprintUsed.WithLabelValues("chrome-100").Inc()
	metrics.ConnectionsPooled.WithLabelValues("example.com").Inc()
	metrics.ConnectionsPooled.WithLabelValues("example.com").Dec()
	metrics.TasksInFlight.Set(5)
	metrics.HeartbeatsSent.Inc()
	metrics.TasksProcessed.WithLabelValues("success").Inc()
	metrics.TasksFailed.WithLabelValues("timeout").Inc()
}

func TestEndpointMetrics_Values(*testing.T) {
	metrics.TasksInFlight.Set(10)

	c := metrics.HeartbeatsSent
	c.Add(1)
}

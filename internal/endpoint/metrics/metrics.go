package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	UpstreamDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "endpoint_upstream_duration_seconds",
			Help:    "Duration of upstream HTTP requests in seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"target_domain", "status"},
	)

	TLSFingerprintUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_tls_fingerprint_used",
			Help: "Count of TLS fingerprints used by ID",
		},
		[]string{"fingerprint"},
	)

	FingerprintDeprecatedUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_fingerprint_deprecated_used",
			Help: "Count of deprecated fingerprints used",
		},
		[]string{"fingerprint"},
	)

	ConnectionsPooled = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "endpoint_connections_pooled",
			Help: "Number of active connections to upstream",
		},
		[]string{"target_host"},
	)

	TasksInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "endpoint_tasks_in_flight",
			Help: "Number of tasks currently being processed",
		},
	)

	HeartbeatsSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_heartbeats_sent_total",
			Help: "Total number of heartbeats sent",
		},
	)

	TasksProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_tasks_processed_total",
			Help: "Total number of tasks processed",
		},
		[]string{"status"},
	)

	TasksFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_tasks_failed_total",
			Help: "Total number of failed tasks by reason",
		},
		[]string{"reason"},
	)

	BytesSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_bytes_sent_total",
			Help: "Total bytes sent to upstream (request body)",
		},
	)

	BytesReceived = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_bytes_received_total",
			Help: "Total bytes received from upstream (response body)",
		},
	)

	TasksQueued = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "endpoint_tasks_queued",
			Help: "Number of tasks waiting in local queue",
		},
	)

	TasksRejected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_tasks_rejected_total",
			Help: "Total number of tasks rejected due to capacity",
		},
	)
)

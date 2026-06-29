// Package metrics provides Prometheus collectors for the endpoint service.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// UpstreamDuration tracks the duration of upstream HTTP requests.
	UpstreamDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "endpoint_upstream_duration_seconds",
			Help:    "Duration of upstream HTTP requests in seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"target_domain", "status"},
	)

	// TLSFingerprintUsed counts TLS fingerprints used by ID.
	TLSFingerprintUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_tls_fingerprint_used_total",
			Help: "Count of TLS fingerprints used by ID",
		},
		[]string{"fingerprint"},
	)

	// FingerprintDeprecatedUsed counts deprecated fingerprints used.
	FingerprintDeprecatedUsed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_fingerprint_deprecated_used_total",
			Help: "Count of deprecated fingerprints used",
		},
		[]string{"fingerprint"},
	)

	// ConnectionsPooled tracks active connections to upstream.
	ConnectionsPooled = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "endpoint_connections_pooled",
			Help: "Number of active connections to upstream",
		},
		[]string{"target_host"},
	)

	// TasksInFlight tracks tasks currently being processed.
	TasksInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "endpoint_tasks_in_flight",
			Help: "Number of tasks currently being processed",
		},
	)

	// HeartbeatsSent tracks total heartbeats sent.
	HeartbeatsSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_heartbeats_sent_total",
			Help: "Total number of heartbeats sent",
		},
	)

	// TasksProcessed tracks total tasks processed by status.
	TasksProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_tasks_processed_total",
			Help: "Total number of tasks processed",
		},
		[]string{"status"},
	)

	// TasksFailed tracks total failed tasks by reason.
	TasksFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "endpoint_tasks_failed_total",
			Help: "Total number of failed tasks by reason",
		},
		[]string{"reason"},
	)

	// BytesSent tracks total bytes sent to upstream.
	BytesSent = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_bytes_sent_total",
			Help: "Total bytes sent to upstream (request body)",
		},
	)

	// BytesReceived tracks total bytes received from upstream.
	BytesReceived = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_bytes_received_total",
			Help: "Total bytes received from upstream (response body)",
		},
	)

	// TasksQueued tracks tasks waiting in local queue.
	TasksQueued = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "endpoint_tasks_queued",
			Help: "Number of tasks waiting in local queue",
		},
	)

	// TasksRejected tracks tasks rejected due to capacity.
	TasksRejected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "endpoint_tasks_rejected_total",
			Help: "Total number of tasks rejected due to capacity",
		},
	)
)

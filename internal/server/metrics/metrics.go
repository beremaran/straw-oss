package metrics

import (
	"sync"

	"github.com/kwilabs/straw-proxy-server/internal/observability/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// RequestsTotal counts the total number of processed requests.
	RequestsTotal *prometheus.CounterVec
	// RequestDuration measures the duration of requests.
	RequestDuration *prometheus.HistogramVec
	// QueueDepth tracks the depth of the request queue (estimated).
	QueueDepth *prometheus.GaugeVec
	// CacheHits counts cache hits.
	CacheHits *prometheus.CounterVec
	// CacheMisses counts cache misses.
	CacheMisses *prometheus.CounterVec
	// ActiveSessions tracks the number of concurrent requests in flight.
	ActiveSessions prometheus.Gauge
	// RateLimitExceeded counts the number of rate limited requests.
	RateLimitExceeded *prometheus.CounterVec

	once sync.Once
)

// Init initializes the server metrics.
func Init() {
	once.Do(func() {
		reg := metrics.GetRegistry()

		RequestsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "relay_requests_total",
				Help: "Total number of relay requests processed",
			},
			[]string{"status", "rule_id", "fingerprint"},
		)

		RequestDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "relay_request_duration_seconds",
				Help:    "Duration of relay requests in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"rule_id", "status"},
		)

		QueueDepth = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "relay_queue_depth",
				Help: "Depth of the relay request queue",
			},
			[]string{"queue_name"},
		)

		CacheHits = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "relay_cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"cache_type"},
		)

		CacheMisses = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "relay_cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"cache_type"},
		)

		ActiveSessions = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "relay_active_sessions",
				Help: "Number of active sessions (concurrent requests)",
			},
		)

		RateLimitExceeded = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "relay_rate_limit_exceeded_total",
				Help: "Total number of requests exceeded rate limit",
			},
			[]string{"quota_key"},
		)

		reg.MustRegister(
			RequestsTotal,
			RequestDuration,
			QueueDepth,
			CacheHits,
			CacheMisses,
			ActiveSessions,
			RateLimitExceeded,
		)
	})
}

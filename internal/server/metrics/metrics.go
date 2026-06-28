package metrics

import (
	"sync"

	"github.com/beremaran/straw/internal/observability/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	RequestsTotal *prometheus.CounterVec

	RequestDuration *prometheus.HistogramVec

	QueueDepth *prometheus.GaugeVec

	CacheHits *prometheus.CounterVec

	CacheMisses *prometheus.CounterVec

	ActiveSessions prometheus.Gauge

	RateLimitExceeded *prometheus.CounterVec

	once sync.Once
)

//nolint:funlen
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

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

func Init() {
	once.Do(initCollectors)
}

func initCollectors() {
	reg := metrics.GetRegistry()

	RequestsTotal = counterVec("relay_requests_total", "Total number of relay requests processed", "status", "rule_id", "fingerprint")
	RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "relay_request_duration_seconds",
			Help:    "Duration of relay requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"rule_id", "status"},
	)
	QueueDepth = gaugeVec("relay_queue_depth", "Depth of the relay request queue", "queue_name")
	CacheHits = counterVec("relay_cache_hits_total", "Total number of cache hits", "cache_type")
	CacheMisses = counterVec("relay_cache_misses_total", "Total number of cache misses", "cache_type")
	ActiveSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "relay_active_sessions",
		Help: "Number of active sessions (concurrent requests)",
	})
	RateLimitExceeded = counterVec("relay_rate_limit_exceeded_total", "Total number of requests exceeded rate limit", "quota_key")

	reg.MustRegister(
		RequestsTotal,
		RequestDuration,
		QueueDepth,
		CacheHits,
		CacheMisses,
		ActiveSessions,
		RateLimitExceeded,
	)
}

func counterVec(name, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: name, Help: help},
		labels,
	)
}

func gaugeVec(name, help string, labels ...string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: name, Help: help},
		labels,
	)
}

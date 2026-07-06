package control

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics is the P0 Control Prometheus metrics surface
// (docs/planning/23-observability.md). Every recording method is safe to
// call on a nil *Metrics: components are constructed and exercised in tests
// without a Metrics instance, so instrumentation must never require one.
//
// Label cardinality is bounded to tenant_id, target_host, route_id, and
// error_code per docs/planning/23; no metric here labels a full URL or a
// worker_id.
type Metrics struct {
	requestsTotal         *prometheus.CounterVec
	requestDuration       *prometheus.HistogramVec
	routingDuration       prometheus.Histogram
	assignmentDuration    prometheus.Histogram
	activeRequests        prometheus.Gauge
	natsRequestDuration   prometheus.Histogram
	natsErrorsTotal       *prometheus.CounterVec
	clickhouseWriteErrors prometheus.Counter
	rateLimitRejections   *prometheus.CounterVec
	quotaRejections       *prometheus.CounterVec
}

// metricLabelTenantID and metricLabelErrorCode are the two dynamic label
// names used across the P0 metric series (docs/planning/23).
const (
	metricLabelTenantID  = "tenant_id"
	metricLabelErrorCode = "error_code"
)

// NewMetrics builds and registers the P0 metric series against reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := newRequestMetrics()
	m.clickhouseWriteErrors, m.rateLimitRejections, m.quotaRejections = newAdmissionMetrics()

	reg.MustRegister(
		m.requestsTotal,
		m.requestDuration,
		m.routingDuration,
		m.assignmentDuration,
		m.activeRequests,
		m.natsRequestDuration,
		m.natsErrorsTotal,
		m.clickhouseWriteErrors,
		m.rateLimitRejections,
		m.quotaRejections,
	)

	return m
}

// newRequestMetrics constructs the dispatch-pipeline series: request
// totals/duration, routing/assignment/NATS durations, active requests, and
// NATS errors.
func newRequestMetrics() *Metrics {
	return &Metrics{
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_requests_total",
			Help: "Total Control REST requests dispatched, by tenant and error code (empty on success).",
		}, []string{metricLabelTenantID, metricLabelErrorCode}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "straw_request_duration_seconds",
			Help:    "Control end-to-end request duration in seconds, by tenant.",
			Buckets: prometheus.DefBuckets,
		}, []string{metricLabelTenantID}),
		routingDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "straw_routing_duration_seconds",
			Help:    "Route evaluation duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		assignmentDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "straw_assignment_duration_seconds",
			Help:    "Worker assignment round-trip duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		activeRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "straw_active_requests",
			Help: "Requests currently dispatched and in flight.",
		}),
		natsRequestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "straw_nats_request_duration_seconds",
			Help:    "NATS assignment request/ack round-trip duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		natsErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_nats_errors_total",
			Help: "NATS transport errors, by error code.",
		}, []string{metricLabelErrorCode}),
	}
}

// newAdmissionMetrics constructs the admission-rejection and
// ClickHouse-write-error series.
func newAdmissionMetrics() (prometheus.Counter, *prometheus.CounterVec, *prometheus.CounterVec) {
	clickhouseWriteErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "straw_clickhouse_write_errors_total",
		Help: "Failed ClickHouse telemetry batch writes.",
	})

	rateLimitRejections := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "straw_rate_limit_rejections_total",
		Help: "Requests rejected by rate limiting, by tenant.",
	}, []string{metricLabelTenantID})

	quotaRejections := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "straw_quota_rejections_total",
		Help: "Requests rejected by quota admission, by tenant.",
	}, []string{metricLabelTenantID})

	return clickhouseWriteErrors, rateLimitRejections, quotaRejections
}

// ObserveRequest records one completed dispatch: the total counter (labeled
// by tenant and the canonical error code string, empty on success) and the
// duration histogram.
func (m *Metrics) ObserveRequest(tenantID, errorCode string, d time.Duration) {
	if m == nil {
		return
	}

	m.requestsTotal.WithLabelValues(tenantID, errorCode).Inc()
	m.requestDuration.WithLabelValues(tenantID).Observe(d.Seconds())
}

// ObserveRouting records one route evaluation's duration.
func (m *Metrics) ObserveRouting(d time.Duration) {
	if m == nil {
		return
	}

	m.routingDuration.Observe(d.Seconds())
}

// ObserveAssignment records one worker assignment round trip's duration.
func (m *Metrics) ObserveAssignment(d time.Duration) {
	if m == nil {
		return
	}

	m.assignmentDuration.Observe(d.Seconds())
}

// IncActiveRequests marks one request as dispatched.
func (m *Metrics) IncActiveRequests() {
	if m == nil {
		return
	}

	m.activeRequests.Inc()
}

// DecActiveRequests marks one dispatched request as complete.
func (m *Metrics) DecActiveRequests() {
	if m == nil {
		return
	}

	m.activeRequests.Dec()
}

// ObserveNATSRequest records one NATS assignment request/ack round trip's
// duration.
func (m *Metrics) ObserveNATSRequest(d time.Duration) {
	if m == nil {
		return
	}

	m.natsRequestDuration.Observe(d.Seconds())
}

// IncNATSError records one NATS transport error by its canonical error code.
func (m *Metrics) IncNATSError(errorCode string) {
	if m == nil {
		return
	}

	m.natsErrorsTotal.WithLabelValues(errorCode).Inc()
}

// IncClickHouseWriteError records one failed ClickHouse batch write.
func (m *Metrics) IncClickHouseWriteError() {
	if m == nil {
		return
	}

	m.clickhouseWriteErrors.Inc()
}

// IncRateLimitRejection records one rate-limit rejection for tenantID.
func (m *Metrics) IncRateLimitRejection(tenantID string) {
	if m == nil {
		return
	}

	m.rateLimitRejections.WithLabelValues(tenantID).Inc()
}

// IncQuotaRejection records one quota-admission rejection for tenantID.
func (m *Metrics) IncQuotaRejection(tenantID string) {
	if m == nil {
		return
	}

	m.quotaRejections.WithLabelValues(tenantID).Inc()
}

// workerStatsSource is implemented by *WorkerRegistry; a narrow interface
// keeps the collector testable against a fake.
type workerStatsSource interface {
	Stats() WorkerRegistryStats
}

// clickHouseQueueSource is implemented by *RequestMetadataWriter.
type clickHouseQueueSource interface {
	QueueDepth() int
}

// RegisterWorkerCollector registers the worker-registry-derived gauges
// (straw_worker_sessions, straw_workers_available,
// straw_worker_heartbeat_age_seconds) against reg. These are computed at
// scrape time from source rather than pushed, since they describe current
// registry state rather than discrete events.
func RegisterWorkerCollector(reg prometheus.Registerer, source workerStatsSource) {
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_worker_sessions",
		Help: "Worker sessions currently registered with Control.",
	}, func() float64 {
		return float64(source.Stats().Sessions)
	}))

	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_workers_available",
		Help: "Worker sessions currently eligible for new assignments.",
	}, func() float64 {
		return float64(source.Stats().Available)
	}))

	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_worker_heartbeat_age_seconds",
		Help: "Age in seconds of the stalest current worker heartbeat.",
	}, func() float64 {
		return source.Stats().MaxHeartbeatAgeSeconds
	}))
}

// RegisterEgressMetricsCollector registers P1 Control-aggregated Egress
// metrics. The gauges are aggregate-only: no worker_id, tenant_id, request_id,
// or URL labels, keeping cardinality bounded by the metric names themselves.
func RegisterEgressMetricsCollector(reg prometheus.Registerer, source workerStatsSource) {
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_egress_active_requests",
		Help: "Active requests currently reported by Egress worker heartbeats.",
	}, func() float64 {
		return float64(source.Stats().ActiveRequests)
	}))

	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_egress_max_concurrency",
		Help: "Aggregate max concurrency currently reported by Egress workers.",
	}, func() float64 {
		return float64(source.Stats().MaxConcurrency)
	}))

	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_egress_available_capacity",
		Help: "Aggregate available capacity currently reported by Egress worker heartbeats.",
	}, func() float64 {
		return float64(source.Stats().AvailableCapacity)
	}))
}

// RegisterClickHouseQueueDepth registers the
// straw_clickhouse_write_queue_depth gauge, computed at scrape time from
// source's buffered event count.
func RegisterClickHouseQueueDepth(reg prometheus.Registerer, source clickHouseQueueSource) {
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_clickhouse_write_queue_depth",
		Help: "Buffered telemetry events awaiting a ClickHouse write.",
	}, func() float64 {
		return float64(source.QueueDepth())
	}))
}

// errorCodeLabel returns the canonical error-code string for code, or "" for
// the zero value (success).
func errorCodeLabel(code ErrorCode) string {
	if code == 0 {
		return ""
	}

	return ErrorRegistry[code].Code
}

package control

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const metricLabelErrorCode = "error_code"

// Metrics contains Control's bounded-cardinality Prometheus metrics.
type Metrics struct {
	requestsTotal      *prometheus.CounterVec
	requestDuration    prometheus.Histogram
	activeRequests     prometheus.Gauge
	routingDuration    prometheus.Histogram
	assignmentDuration prometheus.Histogram
	natsDuration       prometheus.Histogram
	natsErrors         *prometheus.CounterVec
}

// NewMetrics registers and returns the Control metrics set.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		requestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_requests_total", Help: "Completed Control requests by outcome.",
		}, []string{metricLabelErrorCode}),
		requestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "straw_request_duration_seconds", Help: "End-to-end Control request duration.",
		}),
		activeRequests: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "straw_active_requests", Help: "Requests currently running in Control.",
		}),
		routingDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "straw_routing_duration_seconds", Help: "Worker routing duration.",
		}),
		assignmentDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "straw_assignment_duration_seconds", Help: "Worker assignment duration.",
		}),
		natsDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "straw_nats_request_duration_seconds", Help: "NATS request/reply duration.",
		}),
		natsErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_nats_errors_total", Help: "NATS transport errors by code.",
		}, []string{metricLabelErrorCode}),
	}
	registerer.MustRegister(
		m.requestsTotal, m.requestDuration, m.activeRequests, m.routingDuration,
		m.assignmentDuration, m.natsDuration, m.natsErrors,
	)

	return m
}

// IncActiveRequests increments the number of requests currently being handled.
func (m *Metrics) IncActiveRequests() {
	if m != nil {
		m.activeRequests.Inc()
	}
}

// DecActiveRequests decrements the number of requests currently being handled.
func (m *Metrics) DecActiveRequests() {
	if m != nil {
		m.activeRequests.Dec()
	}
}

// ObserveRequest records a completed request and its end-to-end duration.
func (m *Metrics) ObserveRequest(errorCode string, duration time.Duration) {
	if m != nil {
		m.requestsTotal.WithLabelValues(errorCode).Inc()
		m.requestDuration.Observe(duration.Seconds())
	}
}

// ObserveRouting records how long worker selection took.
func (m *Metrics) ObserveRouting(duration time.Duration) {
	if m != nil {
		m.routingDuration.Observe(duration.Seconds())
	}
}

// ObserveAssignment records how long worker assignment took.
func (m *Metrics) ObserveAssignment(duration time.Duration) {
	if m != nil {
		m.assignmentDuration.Observe(duration.Seconds())
	}
}

// ObserveNATSRequest records a NATS request/reply duration.
func (m *Metrics) ObserveNATSRequest(duration time.Duration) {
	if m != nil {
		m.natsDuration.Observe(duration.Seconds())
	}
}

// IncNATSError records a NATS transport error.
func (m *Metrics) IncNATSError(errorCode string) {
	if m != nil {
		m.natsErrors.WithLabelValues(errorCode).Inc()
	}
}

func errorCodeLabel(code ErrorCode) string {
	if code == 0 {
		return ""
	}

	return ErrorRegistry[code].Code
}

type workerStatsSource interface {
	Stats() WorkerRegistryStats
}

// RegisterWorkerCollector exposes aggregate live-worker gauges.
func RegisterWorkerCollector(registerer prometheus.Registerer, source workerStatsSource) {
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_worker_sessions", Help: "Registered worker sessions.",
	}, func() float64 { return float64(source.Stats().Sessions) }))
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_workers_available", Help: "Workers available for assignments.",
	}, func() float64 { return float64(source.Stats().Available) }))
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_worker_heartbeat_age_seconds", Help: "Age of the stalest worker heartbeat.",
	}, func() float64 { return source.Stats().MaxHeartbeatAgeSeconds }))
}

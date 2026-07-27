package egress

import (
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/beremaran/straw-oss/internal/natsx"
	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

const (
	metricLabelOutcome   = "outcome"
	metricLabelDirection = "direction"
	metricLabelCode      = "code"

	outcomeSuccess = "success"
	outcomeError   = "error"

	// directionOut is the body handed to the upstream, directionIn what came
	// back from it. Both are named from the worker's point of view.
	directionOut = "out"
	directionIn  = "in"

	// Egress cannot import internal/control (scripts/verify-dependency-direction.sh),
	// so the canonical error_code strings a label may carry are repeated here.
	// They must stay identical to internal/control/errors.go: a dashboard is
	// expected to select the same code across both jobs.
	errorCodeInvalidRequest            = "invalid_request"
	errorCodeDestinationDenied         = "destination_denied"
	errorCodeHeaderInjectionFailed     = "header_injection_failed"
	errorCodeWorkerDisconnected        = "worker_disconnected"
	errorCodeTransportUnavailable      = "transport_unavailable"
	errorCodeTimeoutExceeded           = "timeout_exceeded"
	errorCodeUnsupportedFingerprint    = "unsupported_fingerprint"
	errorCodeUpstreamDNSFailure        = "upstream_dns_failure"
	errorCodeUpstreamTLSFailure        = "upstream_tls_failure"
	errorCodeUpstreamConnectionRefused = "upstream_connection_refused"
	errorCodeUpstreamConnectTimeout    = "upstream_connect_timeout"
	errorCodeUpstreamReset             = "upstream_reset"
	errorCodeUpstreamProxyFailure      = "upstream_proxy_failure"
	errorCodeBodyRefUnavailable        = "body_ref_unavailable"
	errorCodeBodyTooLarge              = "body_too_large"
	errorCodeExecutorInternalError     = "executor_internal_error"
	errorCodeCancelled                 = "cancelled"
)

// Metrics contains Egress's bounded-cardinality Prometheus metrics.
type Metrics struct {
	assignmentsTotal *prometheus.CounterVec
	requestDuration  prometheus.Histogram
	bytesTotal       *prometheus.CounterVec
	upstreamErrors   *prometheus.CounterVec
	natsErrors       *prometheus.CounterVec
}

// NewMetrics registers and returns the Egress metrics set.
func NewMetrics(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		assignmentsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_egress_assignments_total", Help: "Assignments this worker finished executing, by outcome.",
		}, []string{metricLabelOutcome}),
		requestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "straw_egress_request_duration_seconds", Help: "Decoded outbound request duration measured inside the worker.",
		}),
		bytesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_egress_bytes_total", Help: "Decoded request and response body bytes by direction.",
		}, []string{metricLabelDirection}),
		upstreamErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_egress_upstream_errors_total", Help: "Failed assignments by canonical error code.",
		}, []string{metricLabelCode}),
		natsErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "straw_egress_nats_errors_total", Help: "NATS transport errors by code.",
		}, []string{metricLabelCode}),
	}
	registerer.MustRegister(
		m.assignmentsTotal, m.requestDuration, m.bytesTotal, m.upstreamErrors, m.natsErrors,
	)

	return m
}

// ObserveRequest records one finished decoded HTTP assignment: its outcome,
// its worker-side duration and, on failure, the canonical error code. An empty
// errorCode means success, the same convention Control's straw_requests_total
// uses (docs/public/operations.md).
func (m *Metrics) ObserveRequest(errorCode string, duration time.Duration) {
	if m != nil {
		m.observeOutcome(errorCode)
		m.requestDuration.Observe(duration.Seconds())
	}
}

// ObserveTunnel records one raw CONNECT assignment by the outcome of opening
// the upstream connection. Everything after the open is copied by the worker
// SDK, so neither a duration nor a byte count is observable from here.
func (m *Metrics) ObserveTunnel(errorCode string) {
	if m != nil {
		m.observeOutcome(errorCode)
	}
}

// AddRequestBytes counts the decoded body submitted for outbound execution.
func (m *Metrics) AddRequestBytes(count uint64) {
	if m != nil {
		m.bytesTotal.WithLabelValues(directionOut).Add(float64(count))
	}
}

// AddResponseBytes counts body bytes read back from the upstream.
func (m *Metrics) AddResponseBytes(count uint64) {
	if m != nil {
		m.bytesTotal.WithLabelValues(directionIn).Add(float64(count))
	}
}

// IncNATSError records a NATS transport error.
func (m *Metrics) IncNATSError(errorCode string) {
	if m != nil {
		m.natsErrors.WithLabelValues(errorCode).Inc()
	}
}

func (m *Metrics) observeOutcome(errorCode string) {
	if errorCode == "" {
		m.assignmentsTotal.WithLabelValues(outcomeSuccess).Inc()

		return
	}

	m.assignmentsTotal.WithLabelValues(outcomeError).Inc()
	m.upstreamErrors.WithLabelValues(errorCode).Inc()
}

// upstreamErrorCodes is an allowlist rather than a switch on purpose: it caps
// the label set at codes this repository knows about, so a future protocol
// enum member cannot silently widen the series. An unlisted code is reported
// as an executor internal error, which is what an unrecognized failure is.
var upstreamErrorCodes = map[strawpb.ErrorCode]string{
	strawpb.ErrorCode_ERROR_CODE_INVALID_REQUEST:             errorCodeInvalidRequest,
	strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED:          errorCodeDestinationDenied,
	strawpb.ErrorCode_ERROR_CODE_HEADER_INJECTION_FAILED:     errorCodeHeaderInjectionFailed,
	strawpb.ErrorCode_ERROR_CODE_TIMEOUT_EXCEEDED:            errorCodeTimeoutExceeded,
	strawpb.ErrorCode_ERROR_CODE_UNSUPPORTED_FINGERPRINT:     errorCodeUnsupportedFingerprint,
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_DNS_FAILURE:        errorCodeUpstreamDNSFailure,
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_TLS_FAILURE:        errorCodeUpstreamTLSFailure,
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECTION_REFUSED: errorCodeUpstreamConnectionRefused,
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_CONNECT_TIMEOUT:    errorCodeUpstreamConnectTimeout,
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_RESET:              errorCodeUpstreamReset,
	strawpb.ErrorCode_ERROR_CODE_UPSTREAM_PROXY_FAILURE:      errorCodeUpstreamProxyFailure,
	strawpb.ErrorCode_ERROR_CODE_BODY_REF_UNAVAILABLE:        errorCodeBodyRefUnavailable,
	strawpb.ErrorCode_ERROR_CODE_BODY_TOO_LARGE:              errorCodeBodyTooLarge,
	strawpb.ErrorCode_ERROR_CODE_EXECUTOR_INTERNAL_ERROR:     errorCodeExecutorInternalError,
	strawpb.ErrorCode_ERROR_CODE_CANCELLED:                   errorCodeCancelled,
}

func errorCodeLabel(code strawpb.ErrorCode) string {
	label, ok := upstreamErrorCodes[code]
	if !ok {
		return errorCodeExecutorInternalError
	}

	return label
}

// terminalErrorCode classifies a finished execution from its terminal frame.
// Every return path of executeWithDeployment puts that frame last in the
// returned batch, including when the earlier frames were streamed through
// send, so the tail is the whole outcome. An empty string means the attempt
// ended with an EndFrame.
func terminalErrorCode(frames []*strawpb.StreamFrame) string {
	if len(frames) == 0 {
		return ""
	}

	errFrame := frames[len(frames)-1].GetError()
	if errFrame == nil {
		return ""
	}

	return errorCodeLabel(errFrame.GetCode())
}

type readinessSource interface{ Load() bool }

// RegisterReadinessCollector publishes the readiness flag the /readyz handler
// already reads, so a scrape and a probe can never disagree about whether this
// worker is taking assignments.
func RegisterReadinessCollector(registerer prometheus.Registerer, source readinessSource) {
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_egress_ready", Help: "Whether the worker holds a registered session and accepts assignments.",
	}, func() float64 {
		if source.Load() {
			return 1
		}

		return 0
	}))
}

type sessionStatsSource interface {
	Sessions() uint32
	ActiveRequests() uint32
	ConcurrencyLimit() uint32
}

// RegisterSessionCollector exposes saturation pulled straight from the live
// worker session instead of shadowing counters the SDK already keeps.
func RegisterSessionCollector(registerer prometheus.Registerer, source sessionStatsSource) {
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_egress_sessions_active", Help: "Registered worker sessions currently being served.",
	}, func() float64 { return float64(source.Sessions()) }))
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_egress_active_requests", Help: "Assignments currently executing on this worker.",
	}, func() float64 { return float64(source.ActiveRequests()) }))
	registerer.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "straw_egress_concurrency_limit", Help: "Maximum concurrent assignments admitted for the live session.",
	}, func() float64 { return float64(source.ConcurrencyLimit()) }))
}

// SessionTracker holds the worker serving the current Control-assigned session
// so the pull-style collectors can read live capacity from the SDK runtime
// rather than duplicating it. The SDK builds one worker per session and offers
// no teardown hook, so session liveness is taken from the readiness flag its
// run loop clears the moment a session is lost.
type SessionTracker struct {
	ready  *atomic.Bool
	worker atomic.Pointer[Worker]
	limit  atomic.Uint32
}

// NewSessionTracker returns a tracker bound to the runtime readiness flag.
func NewSessionTracker(ready *atomic.Bool) *SessionTracker {
	return &SessionTracker{ready: ready}
}

// Track records the worker serving a newly registered session together with
// the concurrency ceiling Control negotiated for it.
func (t *SessionTracker) Track(worker *Worker, maxConcurrency uint32) {
	t.worker.Store(worker)
	t.limit.Store(maxConcurrency)
}

// Sessions reports whether a registered session is being served. One worker
// process serves exactly one session at a time, so this is 0 or 1.
func (t *SessionTracker) Sessions() uint32 {
	if t.worker.Load() == nil || t.ready == nil || !t.ready.Load() {
		return 0
	}

	return 1
}

// ActiveRequests reports the assignments the live session is running.
func (t *SessionTracker) ActiveRequests() uint32 {
	worker := t.worker.Load()
	if worker == nil {
		return 0
	}

	return worker.ActiveRequests()
}

// ConcurrencyLimit reports the negotiated ceiling for the live session, which
// is what active requests should be compared against for saturation.
func (t *SessionTracker) ConcurrencyLimit() uint32 {
	return t.limit.Load()
}

// RegisterNATSErrorHandlers routes the NATS client's asynchronous failures
// into straw_egress_nats_errors_total. Losing the connection is this worker
// dropping off the deployment, so it is counted as worker_disconnected; a
// client-side asynchronous error (slow consumer, permissions, expired
// credentials) means the transport itself is unusable and is counted as
// transport_unavailable. A disconnect carrying no error is a deliberate drain
// and is not an error. Both labels come from Control's registry so one query
// spans both jobs.
func RegisterNATSErrorHandlers(conn *natsx.Connection, metrics *Metrics) {
	if conn == nil || conn.Conn == nil {
		return
	}

	conn.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		if err != nil {
			metrics.IncNATSError(errorCodeWorkerDisconnected)
		}
	})

	conn.SetErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, _ error) {
		metrics.IncNATSError(errorCodeTransportUnavailable)
	})
}

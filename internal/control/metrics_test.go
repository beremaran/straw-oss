package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/redis/go-redis/v9"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const metricsTestTenant = "ten1"

func gatherFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}

	t.Fatalf("metric family %s not found", name)

	return nil
}

func counterValue(mf *dto.MetricFamily, labels map[string]string) float64 {
	for _, m := range mf.GetMetric() {
		if labelsMatch(m, labels) {
			return m.GetCounter().GetValue()
		}
	}

	return 0
}

func labelsMatch(m *dto.Metric, labels map[string]string) bool {
	if len(m.GetLabel()) != len(labels) {
		return false
	}

	for _, l := range m.GetLabel() {
		if labels[l.GetName()] != l.GetValue() {
			return false
		}
	}

	return true
}

// TestNewMetricsRegistersP0Series proves every push-based P0 metric name
// from docs/planning/23-observability.md is present once its series has
// been touched at least once. The pull-based worker and
// ClickHouse-queue-depth gauges are registered separately
// (RegisterWorkerCollector, RegisterClickHouseQueueDepth) and are covered by
// their own tests below. A CounterVec/HistogramVec with a dynamic label
// (tenant_id, error_code) carries no series at all until a label
// combination is recorded — this is standard Prometheus client behavior,
// not a defect: Control cannot enumerate tenants in advance.
func TestNewMetricsRegistersP0Series(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.ObserveRequest(metricsTestTenant, "", time.Millisecond)
	m.IncNATSError("transport_unavailable")
	m.IncRateLimitRejection(metricsTestTenant)
	m.IncQuotaRejection(metricsTestTenant)

	want := []string{
		"straw_requests_total",
		"straw_request_duration_seconds",
		"straw_routing_duration_seconds",
		"straw_assignment_duration_seconds",
		"straw_active_requests",
		"straw_nats_request_duration_seconds",
		"straw_nats_errors_total",
		"straw_clickhouse_write_errors_total",
		"straw_rate_limit_rejections_total",
		"straw_quota_rejections_total",
	}

	for _, name := range want {
		gatherFamily(t, reg, name)
	}
}

func TestMetricsObserveRequestAndActiveRequests(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.IncActiveRequests()
	m.IncActiveRequests()

	mf := gatherFamily(t, reg, "straw_active_requests")
	if got := mf.GetMetric()[0].GetGauge().GetValue(); got != 2 {
		t.Fatalf("active_requests = %v, want 2", got)
	}

	m.DecActiveRequests()

	mf = gatherFamily(t, reg, "straw_active_requests")
	if got := mf.GetMetric()[0].GetGauge().GetValue(); got != 1 {
		t.Fatalf("active_requests after Dec = %v, want 1", got)
	}

	m.ObserveRequest(metricsTestTenant, "", 10*time.Millisecond)
	m.ObserveRequest(metricsTestTenant, "route_no_match", 5*time.Millisecond)

	totalMF := gatherFamily(t, reg, "straw_requests_total")
	if got := counterValue(totalMF, map[string]string{metricLabelTenantID: metricsTestTenant, metricLabelErrorCode: ""}); got != 1 {
		t.Fatalf("requests_total success = %v, want 1", got)
	}

	if got := counterValue(totalMF, map[string]string{metricLabelTenantID: metricsTestTenant, metricLabelErrorCode: "route_no_match"}); got != 1 {
		t.Fatalf("requests_total route_no_match = %v, want 1", got)
	}

	durationMF := gatherFamily(t, reg, "straw_request_duration_seconds")

	var sampleCount uint64
	for _, met := range durationMF.GetMetric() {
		sampleCount += met.GetHistogram().GetSampleCount()
	}

	if sampleCount != 2 {
		t.Fatalf("request_duration sample count = %d, want 2", sampleCount)
	}
}

func TestMetricsObserveRoutingAssignmentAndNATS(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.ObserveRouting(1 * time.Millisecond)
	m.ObserveAssignment(2 * time.Millisecond)
	m.ObserveNATSRequest(3 * time.Millisecond)
	m.IncNATSError("transport_unavailable")

	if mf := gatherFamily(t, reg, "straw_routing_duration_seconds"); mf.GetMetric()[0].GetHistogram().GetSampleCount() != 1 {
		t.Fatalf("routing_duration sample count = %d, want 1", mf.GetMetric()[0].GetHistogram().GetSampleCount())
	}

	if mf := gatherFamily(t, reg, "straw_assignment_duration_seconds"); mf.GetMetric()[0].GetHistogram().GetSampleCount() != 1 {
		t.Fatalf("assignment_duration sample count = %d, want 1", mf.GetMetric()[0].GetHistogram().GetSampleCount())
	}

	if mf := gatherFamily(t, reg, "straw_nats_request_duration_seconds"); mf.GetMetric()[0].GetHistogram().GetSampleCount() != 1 {
		t.Fatalf("nats_request_duration sample count = %d, want 1", mf.GetMetric()[0].GetHistogram().GetSampleCount())
	}

	errMF := gatherFamily(t, reg, "straw_nats_errors_total")
	if got := counterValue(errMF, map[string]string{metricLabelErrorCode: "transport_unavailable"}); got != 1 {
		t.Fatalf("nats_errors_total = %v, want 1", got)
	}
}

func TestMetricsRejectionAndWriteErrorCounters(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	m.IncRateLimitRejection(metricsTestTenant)
	m.IncQuotaRejection(metricsTestTenant)
	m.IncClickHouseWriteError()

	if got := counterValue(gatherFamily(t, reg, "straw_rate_limit_rejections_total"), map[string]string{metricLabelTenantID: metricsTestTenant}); got != 1 {
		t.Fatalf("rate_limit_rejections_total = %v, want 1", got)
	}

	if got := counterValue(gatherFamily(t, reg, "straw_quota_rejections_total"), map[string]string{metricLabelTenantID: metricsTestTenant}); got != 1 {
		t.Fatalf("quota_rejections_total = %v, want 1", got)
	}

	mf := gatherFamily(t, reg, "straw_clickhouse_write_errors_total")
	if got := mf.GetMetric()[0].GetCounter().GetValue(); got != 1 {
		t.Fatalf("clickhouse_write_errors_total = %v, want 1", got)
	}
}

// TestMetricsNilSafe proves every recording method tolerates a nil
// *Metrics, since dispatcher/admission/writer components are constructed
// and exercised in many tests without one wired.
func TestMetricsNilSafe(t *testing.T) {
	t.Parallel()

	var m *Metrics

	m.IncActiveRequests()
	m.DecActiveRequests()
	m.ObserveRequest(metricsTestTenant, "", time.Millisecond)
	m.ObserveRouting(time.Millisecond)
	m.ObserveAssignment(time.Millisecond)
	m.ObserveNATSRequest(time.Millisecond)
	m.IncNATSError("x")
	m.IncClickHouseWriteError()
	m.IncRateLimitRejection(metricsTestTenant)
	m.IncQuotaRejection(metricsTestTenant)
}

// TestRateLimitAdmissionRecordsRejectionMetric proves a fail-closed
// rate-limit breach (here forced by an unreachable Redis) increments
// straw_rate_limit_rejections_total for the breaching tenant.
func TestRateLimitAdmissionRecordsRejectionMetric(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	client := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	admission := NewRateLimitAdmission(NewRateLimiter(client, DefaultRateLimitGuardrails(), nil))
	admission.SetMetrics(m)

	cfg := RateLimitConfig{
		TenantID: metricsTestTenant,
		Limits:   []RateLimitRule{{Dimension: RateLimitDimTenant, WindowSeconds: 60, MaxRequests: 1, FailPolicy: RateLimitFailClosed}},
	}

	decision := admission.Check(context.Background(), cfg, RateLimitRequest{TenantID: metricsTestTenant})
	if decision.Allowed {
		t.Fatal("decision.Allowed = true, want false (fail-closed on unreachable redis)")
	}

	if got := counterValue(gatherFamily(t, reg, "straw_rate_limit_rejections_total"), map[string]string{metricLabelTenantID: metricsTestTenant}); got != 1 {
		t.Fatalf("rate_limit_rejections_total = %v, want 1", got)
	}
}

// TestQuotaAdmissionRecordsRejectionMetric proves a fail-closed quota
// admission denial increments straw_quota_rejections_total for the tenant.
func TestQuotaAdmissionRecordsRejectionMetric(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	client := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })

	admission := NewQuotaAdmission(client, nil)
	admission.SetMetrics(m)

	cfg := QuotaConfig{TenantID: metricsTestTenant, RedisFailPolicy: quotaFailPolicyClosed}

	decision := admission.CheckAdmission(context.Background(), cfg)
	if decision.Allowed {
		t.Fatal("decision.Allowed = true, want false (fail-closed on unreachable redis)")
	}

	if got := counterValue(gatherFamily(t, reg, "straw_quota_rejections_total"), map[string]string{metricLabelTenantID: metricsTestTenant}); got != 1 {
		t.Fatalf("quota_rejections_total = %v, want 1", got)
	}
}

type failingRequestEventSink struct{}

var errFailingSink = errors.New("sink write failed")

func (failingRequestEventSink) WriteRequestEvents(context.Context, []RequestEvent) error {
	return errFailingSink
}

// TestRequestMetadataWriterRecordsClickHouseMetrics proves a failed batch
// write increments straw_clickhouse_write_errors_total and that
// straw_clickhouse_write_queue_depth reflects the buffered event count.
func TestRequestMetadataWriterRecordsClickHouseMetrics(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	writer := NewRequestMetadataWriter(failingRequestEventSink{}, 10, 10, time.Hour)
	t.Cleanup(writer.Close)
	writer.SetMetrics(m)

	writer.Enqueue(RequestEvent{RequestID: "req1"})
	writer.Enqueue(RequestEvent{RequestID: "req2"})

	RegisterClickHouseQueueDepth(reg, writer)

	if got := gatherFamily(t, reg, "straw_clickhouse_write_queue_depth").GetMetric()[0].GetGauge().GetValue(); got != 2 {
		t.Fatalf("clickhouse_write_queue_depth = %v, want 2", got)
	}

	err := writer.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want failure from failingRequestEventSink")
	}

	if got := gatherFamily(t, reg, "straw_clickhouse_write_errors_total").GetMetric()[0].GetCounter().GetValue(); got != 1 {
		t.Fatalf("clickhouse_write_errors_total = %v, want 1", got)
	}
}

// TestRegisterWorkerCollectorReflectsRegistryState proves the worker gauges
// are computed from live WorkerRegistry state at scrape time: a registered,
// heartbeat-less session counts toward sessions but not availability, and a
// healthy heartbeat makes it available with a near-zero heartbeat age.
func TestRegisterWorkerCollectorReflectsRegistryState(t *testing.T) {
	t.Parallel()

	h := newRegHarness(t, defaultCred())
	sessionID := h.mustRegister(t, h.signedRegister(workerRegTestWorker1))

	reg := prometheus.NewRegistry()
	RegisterWorkerCollector(reg, h.reg)

	if got := gatherFamily(t, reg, "straw_worker_sessions").GetMetric()[0].GetGauge().GetValue(); got != 1 {
		t.Fatalf("worker_sessions = %v, want 1", got)
	}

	if got := gatherFamily(t, reg, "straw_workers_available").GetMetric()[0].GetGauge().GetValue(); got != 0 {
		t.Fatalf("workers_available before heartbeat = %v, want 0", got)
	}

	ok, err := h.reg.Heartbeat(&strawpb.HeartbeatRequest{
		WorkerId:  workerRegTestWorker1,
		SessionId: sessionID,
		Health:    strawpb.WorkerHealth_WORKER_HEALTH_READY,
	})
	if err != nil || !ok {
		t.Fatalf("Heartbeat() = (%v, %v), want (true, nil)", ok, err)
	}

	if got := gatherFamily(t, reg, "straw_workers_available").GetMetric()[0].GetGauge().GetValue(); got != 1 {
		t.Fatalf("workers_available after heartbeat = %v, want 1", got)
	}

	if got := gatherFamily(t, reg, "straw_worker_heartbeat_age_seconds").GetMetric()[0].GetGauge().GetValue(); got != 0 {
		t.Fatalf("worker_heartbeat_age_seconds right after heartbeat = %v, want 0 (fake clock did not advance)", got)
	}

	h.clock.Advance(5 * time.Second)

	if got := gatherFamily(t, reg, "straw_worker_heartbeat_age_seconds").GetMetric()[0].GetGauge().GetValue(); got != 5 {
		t.Fatalf("worker_heartbeat_age_seconds after 5s advance = %v, want 5", got)
	}
}

package egress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

// TestExecutorCountsSuccessfulRequest proves one executed assignment lands on
// the outcome counter and that both byte directions are counted from the real
// request and response bodies.
func TestExecutorCountsSuccessfulRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(server.Close)

	reg := prometheus.NewRegistry()
	target := rewriteHost(t, server.URL, unitTestHost)
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{unitTestHost: loopbackIP(t, server.URL)},
		Metrics:  NewMetrics(reg),
	})

	frames := exec.Execute(context.Background(), requestStart(target, directPolicy(true)), []byte("payload"), 1, nil)
	if errFrame := terminalErrorOrNil(frames); errFrame != nil {
		t.Fatalf("terminal error = %#v", errFrame)
	}

	assertMetric(t, reg, `straw_egress_assignments_total{outcome="success"} 1`)
	assertMetric(t, reg, `straw_egress_bytes_total{direction="out"} 7`)
	assertMetric(t, reg, `straw_egress_bytes_total{direction="in"} 5`)
	assertMetric(t, reg, "straw_egress_request_duration_seconds_count 1")
}

// TestExecutorCountsUpstreamErrorByCanonicalCode proves a failed assignment is
// labelled with the same error_code string Control publishes for it.
func TestExecutorCountsUpstreamErrorByCanonicalCode(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	exec := NewExecutor(ExecutorOptions{
		Resolver: staticResolver{unitTestHost: netip.MustParseAddr("127.0.0.1")},
		Metrics:  NewMetrics(reg),
	})

	frames := exec.Execute(context.Background(), requestStart("http://unit.test:8080", directPolicy(false)), nil, 1, nil)
	if errFrame := terminalError(t, frames); errFrame.GetCode() != strawpb.ErrorCode_ERROR_CODE_DESTINATION_DENIED {
		t.Fatalf("terminal error code = %v, want destination denied", errFrame.GetCode())
	}

	assertMetric(t, reg, `straw_egress_assignments_total{outcome="error"} 1`)
	assertMetric(t, reg, `straw_egress_upstream_errors_total{code="destination_denied"} 1`)
}

// TestSessionTrackerReportsNoCapacityBeforeRegistration proves the saturation
// gauges are safe to scrape before the first session exists, which is the
// window in which a worker is most likely to be scraped.
func TestSessionTrackerReportsNoCapacityBeforeRegistration(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	ready := &atomic.Bool{}
	RegisterSessionCollector(reg, NewSessionTracker(ready))

	assertMetric(t, reg, "straw_egress_sessions_active 0")
	assertMetric(t, reg, "straw_egress_active_requests 0")
	assertMetric(t, reg, "straw_egress_concurrency_limit 0")
}

func assertMetric(t *testing.T, reg *prometheus.Registry, want string) {
	t.Helper()

	rec := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", nil))

	if !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("scrape missing %q:\n%s", want, rec.Body.String())
	}
}

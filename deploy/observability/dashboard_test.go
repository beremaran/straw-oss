package observability_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStrawOperationalDashboardCoversPlanningSignals(t *testing.T) {
	raw, err := os.ReadFile("grafana/dashboards/straw-operational-overview.json")
	if err != nil {
		t.Fatal(err)
	}

	var dashboard map[string]any
	err = json.Unmarshal(raw, &dashboard)
	if err != nil {
		t.Fatal(err)
	}

	text := string(raw)
	for _, want := range []string{
		"straw_requests_total",
		"straw_request_duration_seconds",
		"straw_routing_duration_seconds",
		"straw_assignment_duration_seconds",
		"straw_active_requests",
		"straw_worker_sessions",
		"straw_workers_available",
		"straw_worker_heartbeat_age_seconds",
		"straw_nats_request_duration_seconds",
		"straw_nats_errors_total",
		"straw_clickhouse_write_queue_depth",
		"straw_clickhouse_write_errors_total",
		"straw_rate_limit_rejections_total",
		"straw_quota_rejections_total",
		"Postgres unavailable",
		"Redis unavailable",
		"NATS unavailable",
		"ClickHouse unavailable",
		"Object storage unavailable",
		"p50 under 100ms",
		"p99 under 500ms",
		"${DS_PROMETHEUS}",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("dashboard missing %q", want)
		}
	}

	for _, forbidden := range []string{"worker_id", "url=", "full_url"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("dashboard includes forbidden high-cardinality/private field %q", forbidden)
		}
	}
}

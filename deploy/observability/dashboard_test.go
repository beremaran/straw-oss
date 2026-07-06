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

func TestGrafanaProvisioningMatchesComposeMounts(t *testing.T) {
	dashboardProvider, err := os.ReadFile("grafana/provisioning/dashboards/straw.yml")
	if err != nil {
		t.Fatal(err)
	}

	datasource, err := os.ReadFile("grafana/provisioning/datasources/prometheus.yml")
	if err != nil {
		t.Fatal(err)
	}

	compose, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}

	prometheusConfig, err := os.ReadFile("prometheus.yml")
	if err != nil {
		t.Fatal(err)
	}

	blackboxConfig, err := os.ReadFile("blackbox.yml")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"/etc/grafana/provisioning/dashboards/straw",
		"Prometheus",
		"http://prometheus:9090",
		"profiles: [\"observability\"]",
		"./deploy/observability/grafana/dashboards:/etc/grafana/provisioning/dashboards/straw:ro",
		"./deploy/observability/grafana/provisioning/datasources:/etc/grafana/provisioning/datasources:ro",
		"./deploy/observability/grafana/provisioning/dashboards:/etc/grafana/provisioning/dashboards:ro",
		"./deploy/observability/prometheus.yml:/etc/prometheus/prometheus.yml:ro",
		"./deploy/observability/blackbox.yml:/etc/blackbox_exporter/config.yml:ro",
		"control:9090",
		"blackbox:9115",
		"backend-outage-probes",
		"tcp_connect",
	} {
		if !strings.Contains(string(dashboardProvider)+string(datasource)+string(compose)+string(prometheusConfig)+string(blackboxConfig), want) {
			t.Fatalf("observability deployment config missing %q", want)
		}
	}
}

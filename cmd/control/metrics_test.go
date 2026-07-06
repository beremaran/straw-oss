package main

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/control"
)

func TestWireMetricsGatesEgressMetrics(t *testing.T) {
	t.Parallel()

	registry := control.NewWorkerRegistry(nil, control.DefaultWorkerTimings(), nil)

	disabled, _ := wireMetrics(config.ControlConfig{}, registry, &clickHouseWriters{})
	if hasMetricFamily(t, disabled, "straw_egress_active_requests") {
		t.Fatal("straw_egress_active_requests registered with egress metrics disabled")
	}

	enabled, _ := wireMetrics(config.ControlConfig{Server: config.ControlServerConfig{EgressMetricsEnabled: true}}, registry, &clickHouseWriters{})
	if !hasMetricFamily(t, enabled, "straw_egress_active_requests") {
		t.Fatal("straw_egress_active_requests missing with egress metrics enabled")
	}
}

func hasMetricFamily(t *testing.T, gatherer prometheus.Gatherer, name string) bool {
	t.Helper()

	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	for _, family := range families {
		if family.GetName() == name {
			return true
		}
	}

	return false
}

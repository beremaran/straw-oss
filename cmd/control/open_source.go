package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/beremaran/straw-oss/v2/internal/config"
	"github.com/beremaran/straw-oss/v2/internal/control"
	"github.com/beremaran/straw-oss/v2/internal/natsx"
)

const defaultRoutePriority = 100

// runDeploymentControl wires the self-hosted single-deployment runtime. NATS
// is its only required backing service; policy and runtime state are local.
func runDeploymentControl(ctx context.Context, cfg config.ControlConfig, natsConn *natsx.Connection) error {
	registry := control.NewDeploymentWorkerRegistry(control.DefaultWorkerTimings(), nil)

	err := control.SetupWorkerDiscoverySubscriptions(ctx, natsConn, registry)
	if err != nil {
		return fmt.Errorf("setup worker discovery: %w", err)
	}

	configCache := newDeploymentConfigCache()

	metricsRegistry := prometheus.NewRegistry()
	metrics := control.NewMetrics(metricsRegistry)
	control.RegisterWorkerCollector(metricsRegistry, registry)

	authenticator := control.NewDeploymentAuthenticator(os.Getenv("STRAW_AUTH_TOKEN"))
	inflight := control.NewInFlightRegistry()
	dispatcher := control.NewDefaultRequestDispatcher(control.RequestDispatcherOptions{
		ConfigCache:                configCache,
		Workers:                    registry,
		NATS:                       natsConn,
		MaxInlineResponseBodyBytes: cfg.Request.MaxInlineResponseBodyBytes,
		MaxFrameDataBytes:          cfg.Transport.MaxFrameDataBytes,
		MaxTimeoutMs:               cfg.Request.MaxTimeoutMs,
		InFlight:                   inflight,
		Metrics:                    metrics,
	})

	requestHandler := control.NewRequestHandler(
		cfg.Request.MaxInlineRequestBodyBytes,
		cfg.Request.MaxTimeoutMs,
		authenticator,
	)
	requestHandler.SetConfigCache(configCache)
	requestHandler.SetDispatcher(dispatcher)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/requests", requestHandler)

	return serveDeployment(ctx, cfg, mux, metricsRegistry)
}

func newDeploymentConfigCache() *control.ConfigCache {
	snapshot := config.NewSnapshot(1)
	snapshot.RoutingRules = []config.RoutingRule{{
		ID:           "default",
		Priority:     defaultRoutePriority,
		Enabled:      true,
		TargetPoolID: config.DefaultPoolID,
	}}
	snapshot.ExecutorPools = []config.ExecutorPool{{
		ID:           config.DefaultPoolID,
		ExecutorType: "egress",
		Enabled:      true,
	}}
	snapshot.FingerprintProfiles = []config.FingerprintProfile{{
		Name:              "chrome_120",
		SupportedByWorker: true,
		Enabled:           true,
		ExecutorType:      "egress",
		ProfileRef:        "chrome_120",
	}}

	return control.NewConfigCache(snapshot)
}

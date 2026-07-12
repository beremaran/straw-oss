package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/beremaran/straw-oss/v2/internal/config"
	"github.com/beremaran/straw-oss/v2/internal/control"
	"github.com/beremaran/straw-oss/v2/internal/natsx"
)

const defaultRoutePriority = 100

var errSharedRuntimeStateURLRequired = errors.New("shared runtime state Redis URL is required")

// runDeploymentControl wires the self-hosted deployment runtime. Runtime state
// is local by default and shared through Redis only in the opt-in HA profile.
func runDeploymentControl(ctx context.Context, cfg config.ControlConfig, natsConn *natsx.Connection) error {
	state, instanceID, err := setupSharedRuntimeState(ctx, cfg)
	if err != nil {
		return err
	}

	registry := control.NewDeploymentWorkerRegistry(control.DefaultWorkerTimings(), nil)
	if state != nil {
		registry = control.NewSharedWorkerRegistry(ctx, control.DefaultWorkerTimings(), nil, state, time.Duration(cfg.RuntimeState.WorkerTTLMS)*time.Millisecond)
	}

	err = control.SetupWorkerDiscoverySubscriptions(ctx, natsConn, registry)
	if err != nil {
		return fmt.Errorf("setup worker discovery: %w", err)
	}

	configCache := newDeploymentConfigCache()

	metricsRegistry := prometheus.NewRegistry()
	metrics := control.NewMetrics(metricsRegistry)
	control.RegisterWorkerCollector(metricsRegistry, registry)

	if state != nil {
		control.RegisterRuntimeStateCollector(metricsRegistry, state)
	}

	inflight := control.NewInFlightRegistry()

	var sticky control.StickyBackend

	if state != nil {
		inflight = control.NewSharedInFlightRegistry(state, instanceID, time.Duration(cfg.RuntimeState.RequestTTLMS)*time.Millisecond, natsConn)

		err = inflight.SetupRemoteCancellation(natsConn)
		if err != nil {
			return fmt.Errorf("setup HA request cancellation: %w", err)
		}

		sticky = control.NewRedisStickyBackend(ctx, state)
	}

	requestHandler := newControlRequestHandler(cfg, natsConn, configCache, registry, sticky, inflight, metrics)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/requests", requestHandler)

	if cfg.RuntimeAdmin.Enabled {
		err = setupRuntimeAdmin(ctx, cfg, natsConn, configCache, registry, inflight, mux)
		if err != nil {
			return err
		}
	}

	var extraReady func() bool
	if state != nil {
		extraReady = state.Available
	}

	return serveDeployment(ctx, cfg, mux, metricsRegistry, extraReady)
}

func newControlRequestHandler(cfg config.ControlConfig, natsConn *natsx.Connection, configCache *control.ConfigCache, registry *control.WorkerRegistry, sticky control.StickyBackend, inflight *control.InFlightRegistry, metrics *control.Metrics) http.Handler {
	authenticator := control.NewDeploymentAuthenticator(os.Getenv("STRAW_AUTH_TOKEN"))
	dispatcher := control.NewDefaultRequestDispatcher(control.RequestDispatcherOptions{
		ConfigCache: configCache, Workers: registry, Sticky: sticky, NATS: natsConn,
		MaxInlineResponseBodyBytes: cfg.Request.MaxInlineResponseBodyBytes,
		MaxFrameDataBytes:          cfg.Transport.MaxFrameDataBytes, MaxTimeoutMs: cfg.Request.MaxTimeoutMs,
		InFlight: inflight, Metrics: metrics,
	})
	requestHandler := control.NewRequestHandler(cfg.Request.MaxInlineRequestBodyBytes, cfg.Request.MaxTimeoutMs, authenticator)
	requestHandler.SetConfigCache(configCache)
	requestHandler.SetDispatcher(dispatcher)

	return requestHandler
}

func setupSharedRuntimeState(ctx context.Context, cfg config.ControlConfig) (*control.RedisRuntimeState, string, error) {
	if cfg.RuntimeState.Backend != "redis" {
		return nil, "", nil
	}

	redisURL := os.Getenv(cfg.RuntimeState.RedisURLEnv)
	if redisURL == "" {
		return nil, "", fmt.Errorf("setup shared runtime state: %w", errSharedRuntimeStateURLRequired)
	}

	operationTimeout := time.Duration(cfg.RuntimeState.OperationTimeoutMS) * time.Millisecond

	client, err := control.NewRESPClient(redisURL, operationTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("setup shared runtime state: %w", err)
	}

	state := control.NewRedisRuntimeState(client, cfg.RuntimeState.KeyPrefix)
	probeCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	err = state.Ping(probeCtx)

	cancel()

	if err != nil {
		return nil, "", fmt.Errorf("setup shared runtime state: %w", err)
	}

	instanceID := os.Getenv(cfg.RuntimeState.InstanceIDEnv)
	if instanceID == "" {
		instanceID, err = control.NewRuntimeInstanceID()
		if err != nil {
			return nil, "", fmt.Errorf("generate Control instance id: %w", err)
		}
	}

	err = natsx.ValidateSubjectToken(instanceID)
	if err != nil {
		return nil, "", fmt.Errorf("invalid Control instance id: %w", err)
	}

	go control.RunInstanceLease(ctx, state, instanceID, time.Duration(cfg.RuntimeState.InstanceTTLMS)*time.Millisecond)

	return state, instanceID, nil
}

func setupRuntimeAdmin(ctx context.Context, cfg config.ControlConfig, natsConn *natsx.Connection, configCache *control.ConfigCache, registry *control.WorkerRegistry, inflight *control.InFlightRegistry, mux *http.ServeMux) error {
	store, err := control.NewNATSConfigStore(natsConn.Conn, cfg.RuntimeAdmin.Bucket, cfg.RuntimeAdmin.HistoryLimit, configCache.Snapshot())
	if err != nil {
		return fmt.Errorf("setup runtime configuration: %w", err)
	}

	adminAuth, err := control.NewAdminAuthenticator(os.Getenv(cfg.RuntimeAdmin.TokenEnv))
	if err != nil {
		return fmt.Errorf("setup runtime administration authorization: %w", err)
	}

	admin, err := control.NewAdminService(store, configCache, registry, inflight, natsConn)
	if err != nil {
		return fmt.Errorf("setup runtime administration: %w", err)
	}

	err = admin.SetupRolloutAcks(natsConn)
	if err != nil {
		return fmt.Errorf("setup runtime rollout status: %w", err)
	}

	err = admin.SetupConfigInvalidation(natsConn)
	if err != nil {
		return fmt.Errorf("setup runtime configuration invalidation: %w", err)
	}

	go admin.RunRepublisher(ctx)

	control.NewAdminHandler(admin, adminAuth).Register(mux)

	return nil
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

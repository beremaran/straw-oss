package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/beremaran/straw-oss/internal/config"
	"github.com/beremaran/straw-oss/internal/control"
	"github.com/beremaran/straw-oss/internal/fingerprint"
	"github.com/beremaran/straw-oss/internal/natsx"
	"github.com/beremaran/straw-oss/internal/objectstore"
	"github.com/beremaran/straw-oss/internal/receipt"
)

const defaultRoutePriority = 100

var (
	errSharedRuntimeStateURLRequired = errors.New("shared runtime state Redis URL is required")
	errS3CredentialsRequired         = errors.New("S3 credentials are required")
)

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

	configCache := newDeploymentConfigCache()
	registry.ApplySnapshot(configCache.Snapshot())

	err = control.SetupWorkerDiscoverySubscriptions(ctx, natsConn, registry)
	if err != nil {
		return fmt.Errorf("setup worker discovery: %w", err)
	}

	receipts, err := setupReceiptTransport(ctx, cfg)
	if err != nil {
		return err
	}

	metricsRegistry, metrics := newMetricsRegistry(registry, state, receipts)

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

	requestHandler, proxyHandler := newControlHandlers(cfg, natsConn, configCache, registry, sticky, inflight, metrics, receipts)

	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/requests", requestHandler)

	if receipts != nil {
		control.NewReceiptHandler(receipts, control.NewDeploymentAuthenticator(os.Getenv("STRAW_AUTH_TOKEN"))).Register(mux)
	}

	if cfg.RuntimeAdmin.Enabled {
		err = setupRuntimeAdmin(ctx, cfg, natsConn, configCache, registry, inflight, mux)
		if err != nil {
			return err
		}
	}

	return serveDeployment(ctx, cfg, proxyHandler.Wrap(mux), metricsRegistry, runtimeReadiness(state))
}

func newMetricsRegistry(registry *control.WorkerRegistry, state *control.RedisRuntimeState, receipts *receipt.Service) (*prometheus.Registry, *control.Metrics) {
	metricsRegistry := prometheus.NewRegistry()
	metrics := control.NewMetrics(metricsRegistry)
	control.RegisterWorkerCollector(metricsRegistry, registry)

	if receipts != nil {
		control.RegisterReceiptCollector(metricsRegistry, receipts)
	}

	if state != nil {
		control.RegisterRuntimeStateCollector(metricsRegistry, state)
	}

	return metricsRegistry, metrics
}

func runtimeReadiness(state *control.RedisRuntimeState) func() bool {
	if state == nil {
		return nil
	}

	return state.Available
}

func newControlHandlers(cfg config.ControlConfig, natsConn *natsx.Connection, configCache *control.ConfigCache, registry *control.WorkerRegistry, sticky control.StickyBackend, inflight *control.InFlightRegistry, metrics *control.Metrics, receipts *receipt.Service) (http.Handler, *control.ProxyHandler) {
	authenticator := control.NewDeploymentAuthenticator(os.Getenv("STRAW_AUTH_TOKEN"))
	dispatcher := control.NewDefaultRequestDispatcher(control.RequestDispatcherOptions{
		ConfigCache: configCache, Workers: registry, Sticky: sticky, NATS: natsConn,
		MaxInlineResponseBodyBytes: cfg.Request.MaxInlineResponseBodyBytes,
		MaxFrameDataBytes:          cfg.Transport.MaxFrameDataBytes, MaxTimeoutMs: cfg.Request.MaxTimeoutMs,
		InFlight: inflight, Metrics: metrics,
		Receipts: receipts,
	})
	requestHandler := control.NewRequestHandler(cfg.Request.MaxInlineRequestBodyBytes, cfg.Request.MaxTimeoutMs, authenticator)
	requestHandler.SetConfigCache(configCache)
	requestHandler.SetDispatcher(dispatcher)

	if receipts != nil {
		requestHandler.SetReceiptPreparer(receipts)
	}

	proxyHandler := control.NewProxyHandler(cfg.Request.MaxInlineRequestBodyBytes, authenticator, dispatcher, dispatcher)

	return requestHandler, proxyHandler
}

func setupReceiptTransport(ctx context.Context, cfg config.ControlConfig) (*receipt.Service, error) {
	storageCfg := cfg.ObjectStorage
	if !storageCfg.Enabled {
		return nil, nil
	}

	signingKey := []byte(os.Getenv(storageCfg.SigningKeyEnv))

	var store objectstore.Store
	if storageCfg.Backend == "local" {
		store = objectstore.Local{Root: storageCfg.LocalDirectory}
	} else {
		accessKey, secretKey := os.Getenv(storageCfg.AccessKeyEnv), os.Getenv(storageCfg.SecretKeyEnv)
		if accessKey == "" || secretKey == "" {
			return nil, fmt.Errorf("setup object storage: %w", errS3CredentialsRequired)
		}

		store = objectstore.S3{Endpoint: storageCfg.Endpoint, Bucket: storageCfg.Bucket, Region: storageCfg.Region, AccessKey: accessKey, SecretKey: secretKey, SessionToken: os.Getenv(storageCfg.SessionTokenEnv), ServerSideEncryption: storageCfg.ServerSideEncryption, KMSKeyID: storageCfg.KMSKeyID}
	}

	service, err := receipt.New(store, receipt.Config{DownloadBaseURL: storageCfg.DownloadBaseURL, SigningKey: signingKey, MaxObjectBytes: storageCfg.MaxObjectBytes, MaxPartBytes: storageCfg.MaxPartBytes, Retention: time.Duration(storageCfg.RetentionSeconds) * time.Second, AssignmentTTL: time.Duration(storageCfg.AssignmentTTLSeconds) * time.Second})
	if err != nil {
		return nil, fmt.Errorf("setup receipt transport: %w", err)
	}

	go service.RunCleanup(ctx, time.Duration(storageCfg.CleanupIntervalSeconds)*time.Second)

	return service, nil
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

	for _, name := range fingerprint.Names() {
		snapshot.FingerprintProfiles = append(snapshot.FingerprintProfiles, config.FingerprintProfile{
			Name:              name,
			SupportedByWorker: true,
			Enabled:           true,
			ExecutorType:      "egress",
			ProfileRef:        name,
			ContractRevision:  fingerprint.ContractRevision,
		})
	}

	return control.NewConfigCache(snapshot)
}

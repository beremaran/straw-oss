// Package main runs the Straw egress worker.
package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/config"
	internalegress "github.com/beremaran/straw/v2/internal/egress"
	"github.com/beremaran/straw/v2/internal/logging"
	"github.com/beremaran/straw/v2/internal/natsx"
	sdkegress "github.com/beremaran/straw/v2/sdk/egress"
)

const (
	defaultConcurrency      = 4
	readHeaderTimeout       = 5 * time.Second
	healthShutdownTimeout   = 5 * time.Second
	healthcheckProbeTimeout = 2 * time.Second
)

var errHealthcheckNotReady = errors.New("healthcheck probe returned non-2xx status")

func main() {
	slog.SetDefault(logging.New("egress"))

	err := run()
	if err != nil {
		slog.Error("egress failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	egressConfig, healthcheck, err := loadEgressConfig()
	if err != nil {
		return err
	}

	if healthcheck {
		return runHealthcheck(egressConfig)
	}

	err = natsx.ValidateServers(egressConfig.NATS.Servers)
	if err != nil {
		return fmt.Errorf("validate nats servers: %w", err)
	}

	natsConn, err := natsx.Connect(natsx.ConnectOptions{
		Servers:             egressConfig.NATS.Servers,
		UserCredentialsFile: egressConfig.NATS.UserCredentialsFile,
		Username:            os.Getenv(egressConfig.NATS.UsernameEnv),
		Password:            os.Getenv(egressConfig.NATS.PasswordEnv),
		ReconnectAttempts:   egressConfig.NATS.ReconnectAttempts,
		ReconnectWait:       time.Duration(egressConfig.NATS.ReconnectWaitMS) * time.Millisecond,
		PingInterval:        time.Duration(egressConfig.NATS.PingIntervalMS) * time.Millisecond,
		MaxPingFailures:     egressConfig.NATS.MaxPingFailures,
	})
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		if natsConn != nil {
			drainErr := natsConn.Drain()
			if drainErr != nil {
				slog.Warn("drain nats connection failed", "error", drainErr)
			}
		}
	}()

	slog.Info("connected to nats", "url", natsConn.ConnectedUrlRedacted())

	return runWorker(ctx, natsConn, egressConfig)
}

// loadEgressConfig parses flags and loads the egress config, returning
// whether -healthcheck was requested (for container healthchecks against the
// distroless image, mirroring cmd/control/main.go).
func loadEgressConfig() (config.EgressConfig, bool, error) {
	configPath := flag.String("config", "", "path to the egress config file")
	healthcheck := flag.Bool("healthcheck", false, "probe the local /readyz endpoint and exit (for container healthchecks)")

	flag.Parse()

	if *configPath == "" {
		return config.DefaultEgress(), *healthcheck, nil
	}

	egressConfig, err := config.LoadEgress(*configPath)
	if err != nil {
		return config.EgressConfig{}, false, fmt.Errorf("load egress config: %w", err)
	}

	return egressConfig, *healthcheck, nil
}

// runHealthcheck probes the local /readyz endpoint on the health port and
// returns nil only on a 2xx.
func runHealthcheck(cfg config.EgressConfig) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/readyz", cfg.HealthPort)

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build healthcheck request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck probe %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: %s status %d", errHealthcheckNotReady, url, resp.StatusCode)
	}

	return nil
}

// serveHealthHTTP starts the /healthz and /readyz server on the health port
// (docs/public/architecture.md, docs/public/architecture.md) and returns a stop function that
// shuts it down.
func serveHealthHTTP(ctx context.Context, cfg config.EgressConfig, ready *atomic.Bool) func() {
	addr := fmt.Sprintf(":%d", cfg.HealthPort)
	server := &http.Server{Addr: addr, Handler: newHealthMux(ready), ReadHeaderTimeout: readHeaderTimeout}

	go func() {
		serveErr := server.ListenAndServe()
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			slog.Error("health server failed", "error", serveErr)
		}
	}()

	slog.Info("health listening", "addr", addr)

	return func() {
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), healthShutdownTimeout)
		defer cancel()

		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			slog.Error("shutdown health server failed", "error", shutdownErr)
		}
	}
}

// buildCapabilities maps the loaded egress static config onto the capability
// claims sent in the worker's RegisterRequest
// (docs/public/architecture.md `egress.capabilities.*`).
func buildCapabilities(cfg config.EgressConfig) sdkegress.Capabilities {
	maxConcurrency := uint32(defaultConcurrency)
	if cfg.Capabilities.MaxConcurrency > 0 {
		maxConcurrency = cfg.Capabilities.MaxConcurrency
	}

	return sdkegress.Capabilities{
		SoftwareVersion: "dev",
		MaxConcurrency:  maxConcurrency,
		AllowedPools: []*strawpb.RegisterRequest_PoolRef{{
			TenantId: config.DefaultDeploymentID,
			PoolId:   config.DefaultPoolID,
		}},
		Tags:                         cfg.Capabilities.Tags,
		Countries:                    cfg.Capabilities.Countries,
		Regions:                      cfg.Capabilities.Regions,
		IPTypes:                      cfg.Capabilities.IPTypes,
		SupportedIngressModes:        cfg.Capabilities.SupportedIngressModes,
		SupportedFingerprintProfiles: internalegress.SupportedFingerprintProfiles(),
	}
}

func runWorker(ctx context.Context, natsConn *natsx.Connection, cfg config.EgressConfig) error {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("generate worker identity: %w", err)
	}

	id := sdkegress.Identity{
		WorkerID:     cfg.WorkerID,
		ExecutorType: "egress",
		PrivateKey:   priv,
	}

	caps := buildCapabilities(cfg)

	heartbeatInterval := time.Duration(cfg.HeartbeatIntervalMs) * time.Millisecond

	pool := cfg.UpstreamConnectionPool
	executor := internalegress.NewExecutor(internalegress.ExecutorOptions{
		HTTP2Enabled:     cfg.HTTP2.Enabled,
		FallbackCacheTTL: time.Duration(cfg.HTTP2.FallbackCacheTTLMS) * time.Millisecond,
		Pool: internalegress.UpstreamConnectionPoolOptions{
			Enabled:             pool.Enabled,
			MaxIdleConnsPerHost: pool.MaxIdleConnsPerHost,
			IdleTimeout:         time.Duration(pool.IdleTimeoutMS) * time.Millisecond,
			MaxLifetime:         time.Duration(pool.MaxLifetimeMS) * time.Millisecond,
		},
	})

	ready := &atomic.Bool{}

	stopHealth := serveHealthHTTP(ctx, cfg, ready)
	defer stopHealth()

	slog.Info("starting run loop", "worker_id", cfg.WorkerID, "heartbeat_interval", heartbeatInterval.String())

	err = runSDKWorker(ctx, natsConn, id, caps, executor, heartbeatInterval, ready)
	if err != nil {
		return fmt.Errorf("egress run loop: %w", err)
	}

	return nil
}

var runSDKWorker = func(ctx context.Context, natsConn *natsx.Connection, id sdkegress.Identity, caps sdkegress.Capabilities, executor *internalegress.Executor, heartbeatInterval time.Duration, ready *atomic.Bool) error {
	return sdkegress.Run(ctx, natsConn, id, caps, heartbeatInterval, ready, func(sessionID string, maxConcurrency uint32) (sdkegress.AssignmentServer, error) {
		return internalegress.NewWorker(natsConn, internalegress.Identity(id), executor, sessionID, maxConcurrency)
	})
}

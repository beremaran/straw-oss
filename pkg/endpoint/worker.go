package endpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	endpointhttp "github.com/beremaran/straw/internal/endpoint/http"
	endpointtls "github.com/beremaran/straw/internal/endpoint/tls"
	endpointtransport "github.com/beremaran/straw/internal/endpoint/transport"
	"github.com/beremaran/straw/internal/endpoint/update"
	"github.com/beremaran/straw/internal/observability/logging"
	obsmetrics "github.com/beremaran/straw/internal/observability/metrics"
	"github.com/beremaran/straw/internal/observability/tracing"
	"github.com/beremaran/straw/pkg/broker"
)

var (
	// Version is the build version of the endpoint worker.
	Version = "dev"
)

// Worker wraps the endpoint components and manages their execution lifecycle.
type Worker struct {
	cfg      *config.EndpointConfig
	executor RequestExecutor
}

// WorkerOption defines a functional option for configuring a Worker.
type WorkerOption func(*Worker)

// WithRequestExecutor configures the Worker to use a custom RequestExecutor.
// If not provided, a default TLS-enabled HTTP client is used.
func WithRequestExecutor(executor RequestExecutor) WorkerOption {
	return func(w *Worker) {
		w.executor = executor
	}
}

// NewWorker initializes a new Worker with the given config and options.
func NewWorker(cfg *config.EndpointConfig, opts ...WorkerOption) *Worker {
	w := &Worker{
		cfg: cfg,
	}
	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Run loads the endpoint configuration from the environment and starts a default worker.
// It listens for system signals (SIGINT, SIGTERM) to initiate a graceful shutdown.
func Run() error {
	cfg, err := config.LoadEndpointConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	return w.Start(ctx)
}

// RunWithConfig starts the worker with the provided configuration.
// It listens for system signals (SIGINT, SIGTERM) to initiate a graceful shutdown.
func RunWithConfig(cfg *config.EndpointConfig) error {
	w := NewWorker(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	return w.Start(ctx)
}

// Start starts all background components of the worker and blocks until the context is canceled.
func (w *Worker) Start(ctx context.Context) error {
	cfg := w.cfg

	logger := logging.SetupLogger(logging.Config{
		Level:   cfg.Observability.LogLevel,
		Format:  cfg.Observability.LogFormat,
		Service: "endpoint",
		Version: Version,
	})

	logger.Info("starting endpoint worker",
		"endpoint_id", cfg.ID,
		"version", Version,
		"concurrency_limit", cfg.ConcurrencyLimit,
	)

	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-endpoint", Version)
	if err != nil {
		logger.Warn("failed to initialize tracer provider", "error", err)
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			err := shutdownTracer(shutdownCtx)
			if err != nil {
				logger.Error("failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	var pooledTransport *endpointtransport.PooledTransport
	executor := w.executor

	if executor == nil {
		registry := fingerprint.DefaultRegistry()
		logger.Info("fingerprint registry initialized", "count", registry.Count())

		poolConfig := endpointtransport.DefaultPoolConfig().
			WithMaxPoolHosts(cfg.MaxPoolHosts).
			WithIdleConnsPerHost(cfg.IdleConnsPerHost).
			WithIdleConnTimeout(cfg.IdleConnTimeout)

		pooledTransport = endpointtransport.NewPooledTransport(poolConfig, func(ctx context.Context, network, addr, fp string) (net.Conn, error) {
			return endpointtls.Dial(ctx, network, addr, fp)
		})
		defer func() { _ = pooledTransport.Close() }()

		httpClient := endpointhttp.NewClient(
			registry,
			pooledTransport,
			endpointhttp.WithEndpointID(cfg.ID),
			endpointhttp.WithDefaultTimeout(30*time.Second),
		)
		defer func() { _ = httpClient.Close() }()
		executor = httpClient
	}

	mqBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)

	err = mqBroker.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to message broker: %w", err)
	}
	defer func() { _ = mqBroker.Close() }()
	logger.Info("connected to message broker")

	hbSender := NewHeartbeatSender(
		mqBroker,
		cfg.ID,
		WithHeartbeatVersion(Version),
		WithHeartbeatTags(cfg.Tags),
		WithHeartbeatInterval(10*time.Second),
		WithHeartbeatLogger(logger.WithGroup("heartbeat")),
	)

	resultPublisher := NewPublisher(
		mqBroker,
		WithPublisherLogger(logger.WithGroup("publisher")),
	)

	var updateChecker *update.Checker
	if cfg.SelfUpdateEnabled && cfg.SelfUpdateURL != "" {
		installer := update.NewInstaller(
			update.WithInstallerLogger(logger.WithGroup("installer")),
		)

		updateChecker = update.NewChecker(
			cfg.SelfUpdateURL,
			Version,
			update.WithCheckInterval(cfg.SelfUpdateInterval),
			update.WithCheckerLogger(logger.WithGroup("update")),
			update.WithUpdateCallback(func(r *update.Result) bool {
				logger.Info("starting auto-update", "new_version", r.NewVersion)

				updateCtx, msgCancel := context.WithTimeout(ctx, 5*time.Minute)
				defer msgCancel()

				err := installer.Install(updateCtx, &update.VersionManifest{
					Version: r.NewVersion,
					URL:     r.DownloadURL,
					SHA256:  r.Checksum,
				})
				if err != nil {
					logger.Error("failed to install update", "error", err)

					return false
				}

				logger.Info("update installed, restarting...")
				err = installer.ReplaceAndRestart()
				if err != nil {
					logger.Error("failed to restart", "error", err)

					return false
				}

				return true
			}),
		)
	}

	taskConsumer := NewConsumer(
		mqBroker,
		executor,
		[]byte(cfg.Security.HMACSecret),
		cfg.ID,
		WithConcurrencyLimit(cfg.ConcurrencyLimit),
		WithLogger(logger.WithGroup("consumer")),
		WithResultHandler(resultPublisher.Handler()),
	)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		hbSender.Start(ctx)
	}()

	if updateChecker != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			updateChecker.Start(ctx)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := taskConsumer.Start(ctx)
		if err != nil {
			logger.Error("consumer stopped with error", "error", err)
		}
	}()

	obsmetrics.Init()
	healthServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
		Handler: setupHealthHandler(),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("starting health/metrics server", "addr", healthServer.Addr)
		err := healthServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server failed", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("health server shutdown error", "error", err)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("shutdown complete")
	case <-time.After(30 * time.Second):
		logger.Warn("shutdown timed out, forcing exit")
	}

	return nil
}

func setupHealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", obsmetrics.Handler())
	obsmetrics.RegisterPprof(mux)

	return mux
}

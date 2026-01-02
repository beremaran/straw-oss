package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/consumer"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/heartbeat"
	endpointhttp "github.com/kwilabs/straw-proxy-server/internal/endpoint/http"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/publisher"
	endpointtls "github.com/kwilabs/straw-proxy-server/internal/endpoint/tls"
	endpointtransport "github.com/kwilabs/straw-proxy-server/internal/endpoint/transport"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/update"
	"github.com/kwilabs/straw-proxy-server/internal/observability/logging"
	"github.com/kwilabs/straw-proxy-server/internal/observability/metrics"
	"github.com/kwilabs/straw-proxy-server/internal/observability/tracing"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Load Configuration
	cfg, err := config.LoadEndpointConfig("")
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// 2. Setup Logger
	logger := logging.SetupLogger(logging.Config{
		Level:   cfg.Core.LogLevel,
		Format:  cfg.Core.LogFormat,
		Service: "straw-endpoint",
		Version: "dev", // TODO: Inject version
	})

	logger.Info("starting endpoint worker",
		"endpoint_id", cfg.ID,
		"version", "dev", // TODO: Inject version at build time
		"concurrency_limit", cfg.ConcurrencyLimit,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	// Initialize OpenTelemetry
	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-endpoint", "dev") // TODO: Version injection
	if err != nil {
		logger.Warn("failed to initialize tracer provider", "error", err)
	} else {
		defer func() {
			if err := shutdownTracer(context.Background()); err != nil {
				logger.Error("failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	// 3. Initialize Components

	// fingerprint registry (built-in presets match design doc)
	registry := fingerprint.DefaultRegistry()
	logger.Info("fingerprint registry initialized", "count", registry.Count())

	// Connection Pool
	poolConfig := endpointtransport.DefaultPoolConfig().
		WithMaxPoolHosts(cfg.MaxPoolHosts).
		WithIdleConnsPerHost(cfg.IdleConnsPerHost).
		WithIdleConnTimeout(cfg.IdleConnTimeout)

	pooledTransport := endpointtransport.NewPooledTransport(poolConfig, func(ctx context.Context, network, addr, fp string) (net.Conn, error) {
		return endpointtls.Dial(ctx, network, addr, fp)
	})
	defer func() { _ = pooledTransport.Close() }()

	// http client
	httpClient := endpointhttp.NewClient(
		registry,
		pooledTransport,
		endpointhttp.WithEndpointID(cfg.ID),
		endpointhttp.WithDefaultTimeout(30*time.Second), // sane default
	)
	defer func() { _ = httpClient.Close() }()

	// broker
	mqBroker := broker.NewRabbitMQBroker(
		broker.Addrs(cfg.Core.RabbitMQURL),
		broker.PrefetchCount(cfg.ConcurrencyLimit), // Prefetch matches concurrency limit
	)

	if err := mqBroker.Connect(); err != nil {
		return fmt.Errorf("failed to connect to message broker: %w", err)
	}
	defer func() { _ = mqBroker.Close() }()
	logger.Info("connected to message broker")

	// heartbeat sender
	hbSender := heartbeat.New(
		mqBroker,
		cfg.ID,
		heartbeat.WithVersion("dev"),
		heartbeat.WithTags(cfg.Tags),
		heartbeat.WithInterval(10*time.Second),
		heartbeat.WithLogger(logger.WithGroup("heartbeat")),
		// ActiveTasks callback will be wired to consumer
	)

	// Publisher
	resultPublisher := publisher.New(
		mqBroker,
		publisher.WithLogger(logger.WithGroup("publisher")),
	)

	// self-update checker
	var updateChecker *update.Checker
	if cfg.SelfUpdateEnabled && cfg.SelfUpdateURL != "" {
		// Initialize installer
		installer := update.NewInstaller(
			update.WithInstallerLogger(logger.WithGroup("installer")),
		)

		updateChecker = update.NewChecker(
			cfg.SelfUpdateURL,
			"dev", // current version
			update.WithCheckInterval(cfg.SelfUpdateInterval),
			update.WithCheckerLogger(logger.WithGroup("update")),
			update.WithUpdateCallback(func(r *update.UpdateResult) bool {
				logger.Info("starting auto-update", "new_version", r.NewVersion)

				// Create a separate context for update to ensure it completes even if main ctx is cancelled
				updateCtx, msgCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer msgCancel()

				if err := installer.Install(updateCtx, &update.VersionManifest{
					Version: r.NewVersion,
					URL:     r.DownloadURL,
					SHA256:  r.Checksum,
				}); err != nil {
					logger.Error("failed to install update", "error", err)
					return false
				}

				// Restart
				logger.Info("update installed, restarting...")
				if err := installer.ReplaceAndRestart(); err != nil {
					logger.Error("failed to restart", "error", err)
					return false
				}
				return true
			}),
		)
	}

	// consumer
	taskConsumer := consumer.New(
		mqBroker,
		httpClient,
		[]byte(cfg.Security.HMACSecret),
		cfg.ID,
		consumer.WithConcurrencyLimit(cfg.ConcurrencyLimit),
		consumer.WithLogger(logger.WithGroup("consumer")),
		consumer.WithResultHandler(resultPublisher.Handler()),
	)

	// 4. Start Background Services
	var wg sync.WaitGroup

	// Start Heartbeat
	wg.Add(1)
	go func() {
		defer wg.Done()
		hbSender.Start(ctx)
	}()

	// Start Update Checker
	if updateChecker != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			updateChecker.Start(ctx)
		}()
	}

	// Start Consumer
	// Consumer.Start blocks, so run in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := taskConsumer.Start(ctx); err != nil {
			logger.Error("consumer stopped with error", "error", err)
			cancel() // Stop everything else if consumer fails
		}
	}()

	// 5. Start Health Check Server
	metrics.Init()
	healthServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
		Handler: setupHealthHandler(),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("starting health/metrics server", "addr", healthServer.Addr)
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server failed", "error", err)
		}
	}()

	// Wait for shutdown signal (handled by ctx cancellation)
	<-ctx.Done()
	logger.Info("shutting down...")

	// Graceful shutdown of health server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("health server shutdown error", "error", err)
	}

	// Stop components (Consumer.Start returns on ctx done, others have Stop methods)
	// Consumer stops when ctx is cancelled (checked in Start implementation).
	// Heartbeat stops when ctx is cancelled (checked in Start implementation which passes derived ctx to run).
	// But explicit Stop calls are good practice if they have them.
	// Consumer has Stop() which cancels its internal context.
	// Heartbeat has Stop().
	// UpdateChecker has Stop().
	// However, we used the main `ctx` to run them, or passed `ctx` to their Start methods.
	// Reading outlines:
	// Consumer.Start(ctx) -> uses ctx.
	// Heartbeat.Start(ctx) -> uses ctx.
	// Checker.Start(ctx) -> uses ctx.
	// So simply cancelling the main `ctx` (which we did at defer cancel() or triggering it) should stop them.
	// We wait for them to finish.
	// Actually we are waiting on wg.Wait().
	// If `healthServer` is in generated goroutine, it needs to be stopped explicitly via Shutdown on ctx done, which we did.

	// Wait for all background routines to exit
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("shutdown complete")
	case <-time.After(30 * time.Second): // Default shutdown timeout
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
	// Metrics handler
	mux.Handle("/metrics", metrics.Handler())
	return mux
}

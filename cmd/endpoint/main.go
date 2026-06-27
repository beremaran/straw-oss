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

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/endpoint/consumer"
	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/internal/endpoint/heartbeat"
	endpointhttp "github.com/beremaran/straw/internal/endpoint/http"
	"github.com/beremaran/straw/internal/endpoint/publisher"
	endpointtls "github.com/beremaran/straw/internal/endpoint/tls"
	endpointtransport "github.com/beremaran/straw/internal/endpoint/transport"
	"github.com/beremaran/straw/internal/endpoint/update"
	"github.com/beremaran/straw/internal/observability/logging"
	"github.com/beremaran/straw/internal/observability/metrics"
	"github.com/beremaran/straw/internal/observability/tracing"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadEndpointConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	logger := logging.SetupLogger(logging.Config{
		Level:   cfg.Observability.LogLevel,
		Format:  cfg.Observability.LogFormat,
		Service: "endpoint",
		Version: "dev",
	})

	logger.Info("starting endpoint worker",
		"endpoint_id", cfg.ID,
		"version", "dev",
		"concurrency_limit", cfg.ConcurrencyLimit,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		logger.Info("received shutdown signal", "signal", sig)
		cancel()
	}()

	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-endpoint", "dev")
	if err != nil {
		logger.Warn("failed to initialize tracer provider", "error", err)
	} else {
		defer func() {
			if err := shutdownTracer(context.Background()); err != nil {
				logger.Error("failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	registry := fingerprint.DefaultRegistry()
	logger.Info("fingerprint registry initialized", "count", registry.Count())

	poolConfig := endpointtransport.DefaultPoolConfig().
		WithMaxPoolHosts(cfg.MaxPoolHosts).
		WithIdleConnsPerHost(cfg.IdleConnsPerHost).
		WithIdleConnTimeout(cfg.IdleConnTimeout)

	pooledTransport := endpointtransport.NewPooledTransport(poolConfig, func(ctx context.Context, network, addr, fp string) (net.Conn, error) {
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

	mqBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)

	if err := mqBroker.Connect(); err != nil {
		return fmt.Errorf("failed to connect to message broker: %w", err)
	}
	defer func() { _ = mqBroker.Close() }()
	logger.Info("connected to message broker")

	hbSender := heartbeat.New(
		mqBroker,
		cfg.ID,
		heartbeat.WithVersion("dev"),
		heartbeat.WithTags(cfg.Tags),
		heartbeat.WithInterval(10*time.Second),
		heartbeat.WithLogger(logger.WithGroup("heartbeat")),
	)

	resultPublisher := publisher.New(
		mqBroker,
		publisher.WithLogger(logger.WithGroup("publisher")),
	)

	var updateChecker *update.Checker
	if cfg.SelfUpdateEnabled && cfg.SelfUpdateURL != "" {
		installer := update.NewInstaller(
			update.WithInstallerLogger(logger.WithGroup("installer")),
		)

		updateChecker = update.NewChecker(
			cfg.SelfUpdateURL,
			"dev",
			update.WithCheckInterval(cfg.SelfUpdateInterval),
			update.WithCheckerLogger(logger.WithGroup("update")),
			update.WithUpdateCallback(func(r *update.Result) bool {
				logger.Info("starting auto-update", "new_version", r.NewVersion)

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

				logger.Info("update installed, restarting...")
				if err := installer.ReplaceAndRestart(); err != nil {
					logger.Error("failed to restart", "error", err)
					return false
				}
				return true
			}),
		)
	}

	taskConsumer := consumer.New(
		mqBroker,
		httpClient,
		[]byte(cfg.Security.HMACSecret),
		cfg.ID,
		consumer.WithConcurrencyLimit(cfg.ConcurrencyLimit),
		consumer.WithLogger(logger.WithGroup("consumer")),
		consumer.WithResultHandler(resultPublisher.Handler()),
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
		if err := taskConsumer.Start(ctx); err != nil {
			logger.Error("consumer stopped with error", "error", err)
			cancel()
		}
	}()

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
	mux.Handle("/metrics", metrics.Handler())
	metrics.RegisterPprof(mux)
	return mux
}

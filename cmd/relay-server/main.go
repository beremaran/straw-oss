// Package main is the entry point for the relay server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/kwilabs/straw-proxy-server/internal/infra/postgres"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/observability/logging"
	"github.com/kwilabs/straw-proxy-server/internal/observability/metrics"
	"github.com/kwilabs/straw-proxy-server/internal/observability/tracing"
	"github.com/kwilabs/straw-proxy-server/internal/server"
	"github.com/kwilabs/straw-proxy-server/internal/server/admin"
	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/kwilabs/straw-proxy-server/internal/service/endpoint"
	"github.com/kwilabs/straw-proxy-server/internal/service/filter"
	"github.com/kwilabs/straw-proxy-server/internal/service/orchestrator"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	// 1. Load Configuration
	cfg, err := config.LoadServerConfig("")
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// 2. Initialize Infrastructure
	logger := logging.SetupLogger(logging.Config{
		Level:   cfg.Core.LogLevel,
		Format:  cfg.Core.LogFormat,
		Service: "straw-relay-server",
		Version: Version,
	})
	slog.SetDefault(logger)
	ctx := context.Background()

	// Initialize OpenTelemetry
	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-relay-server", Version)
	if err != nil {
		slog.Warn("Failed to initialize tracer provider", "error", err)
	} else {
		defer func() {
			if err := shutdownTracer(ctx); err != nil {
				slog.Error("Failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	// Circuit Breakers
	pgBreaker := circuitbreaker.New(circuitbreaker.Config{
		Name:             "postgres",
		FailureThreshold: 5,
		ResetTimeout:     30 * time.Second,
	})
	redisBreaker := circuitbreaker.New(circuitbreaker.Config{
		Name:             "redis",
		FailureThreshold: 10,
		ResetTimeout:     10 * time.Second,
	})
	rabbitBreaker := circuitbreaker.New(circuitbreaker.Config{
		Name:             "rabbitmq",
		FailureThreshold: 5,
		ResetTimeout:     20 * time.Second,
	})

	// Postgres
	pgClient, err := postgres.NewClient(ctx, cfg.Core.PostgresDSN, pgBreaker)
	if err != nil {
		slog.Error("Failed to connect to Postgres", "error", err)
		os.Exit(1)
	}
	defer pgClient.Close()

	// 3. Run Migrations if enabled
	if cfg.Core.DBAutoMigrate {
		slog.Info("Applying pending migrations...")
		if err := postgres.RunEmbeddedMigrations(cfg.Core.PostgresDSN); err != nil {
			slog.Error("Failed to run migrations", "error", err)
			os.Exit(1)
		}
		slog.Info("Migrations applied successfully!")
	}

	// Redis
	redisClient, err := redis.NewClient(cfg.Core.RedisAddr, redisBreaker)
	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	// 4. Initialize Services
	apiKeyRepo := postgres.NewApiKeyRepository(pgClient)
	authCache := auth.NewAuthCache(redisClient, 5*time.Minute)
	authService := auth.NewAuthService(apiKeyRepo, authCache)

	sessionStore := session.NewRedisStore(redisClient)
	sessionService := session.NewService(sessionStore)

	ruleRepo := postgres.NewRoutingRuleRepository(pgClient)
	ruleCache := router.NewRuleCache(redisClient.Client, 10*time.Minute)
	matcher := router.NewMatcher(ruleRepo, ruleCache)

	// Rate Limiter
	rateLimiter := ratelimit.NewRateLimiter(redisClient)

	// Filter Service
	abpMatcher := filter.NewABPMatcher(redisClient, filter.ABPMatcherConfig{
		UpdateInterval: 24 * time.Hour,
	})
	// Run auto update
	go abpMatcher.StartAutoUpdate(ctx)

	filterService := filter.NewService(abpMatcher)

	// Orchestrator Dependencies
	endpointHealthStore := redis.NewEndpointHealthStore(redisClient)
	endpointSelector := orchestrator.NewSimpleEndpointSelector(endpointHealthStore)

	// Here creating a shared broker instance earlier would be better.
	// Let's create the broker connection HERE instead of later for Admin.

	// NATS Broker
	// We use the NatsURL from config.
	// CircuitBreaker option is ignored by NatsBroker for now as NATS has built-in reconnection logic,
	// but we can pass it if we implement wrap logic later.
	natsBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.Core.NatsURL),
		broker.Token(cfg.Core.NatsToken),
	)
	if err := natsBroker.Connect(); err != nil {
		slog.Error("Failed to connect to message broker", "error", err)
		os.Exit(1)
	}
	defer func() { _ = natsBroker.Close() }()

	// Declare required exchanges before endpoints can connect
	// These exchanges must exist for endpoints to successfully publish/consume messages
	if err := natsBroker.DeclareExchange(ctx, "heartbeats", "fanout"); err != nil {
		slog.Error("Failed to declare heartbeats exchange", "error", err)
		os.Exit(1)
	}
	if err := natsBroker.DeclareExchange(ctx, "tasks", "direct"); err != nil {
		slog.Error("Failed to declare tasks exchange", "error", err)
		os.Exit(1)
	}
	if err := natsBroker.DeclareExchange(ctx, "results", "direct"); err != nil {
		slog.Error("Failed to declare results exchange", "error", err)
		os.Exit(1)
	}
	slog.Info("NATS streams declared successfully", "streams", []string{"heartbeats", "tasks", "results"})

	// Declare heartbeats queue before binding it
	if err := natsBroker.DeclareQueue(ctx, "heartbeats"); err != nil {
		slog.Error("Failed to declare heartbeats queue", "error", err)
		os.Exit(1)
	}

	// Bind heartbeats queue to heartbeats exchange
	// For fanout exchanges, routing key is ignored (empty string)
	if err := natsBroker.BindQueue(ctx, "heartbeats", "heartbeats", ""); err != nil {
		slog.Error("Failed to bind heartbeats queue to exchange", "error", err)
		os.Exit(1)
	}
	slog.Info("NATS heartbeats consumer bound to stream")

	publisher := orchestrator.NewPublisher(natsBroker, endpointSelector, []byte(cfg.Security.HMACSecret), rabbitBreaker)
	consumer := orchestrator.NewConsumer(natsBroker)

	retryExecutor := orchestrator.NewRetryExecutor(
		publisher,
		consumer,
		endpointSelector, // SimpleEndpointSelector implements PoolManager
		natsBroker,
		[]byte(cfg.Security.HMACSecret),
	)

	// Start Retry Executor response listener
	if err := retryExecutor.Start(ctx); err != nil {
		slog.Error("Failed to start retry executor", "error", err)
		os.Exit(1)
	}

	// Pre-warm Cache / Load Rules
	if err := matcher.LoadRules(ctx); err != nil {
		slog.Warn("Failed to load initial routing rules", "error", err)
	}
	// Start auto-refresh
	matcher.StartAutoRefresh(ctx, 1*time.Minute)

	// 5. Initialize Server
	srv := server.New(*cfg, authService, sessionService, matcher, rateLimiter, filterService, retryExecutor)

	// 6. Initialize Admin Server Dependencies
	// Health Service
	healthService := endpoint.NewHealthService(natsBroker, endpointHealthStore)
	if err := healthService.Start(ctx); err != nil {
		slog.Warn("Failed to start health service", "error", err)
	}
	defer healthService.Stop()

	// Admin Server
	adminSrv := admin.New(*cfg, pgClient, redisClient, healthService, natsBroker)

	// 7. Start Servers
	go func() {
		if err := srv.Start(); err != nil {
			slog.Error("Server shutting down", "error", err)
		}
	}()

	go func() {
		if err := adminSrv.Start(); err != nil {
			slog.Error("Admin Server shutting down", "error", err)
		}
	}()

	// Metrics Server
	var metricsSrv *http.Server
	if cfg.Observability.MetricsEnabled {
		metrics.Init()
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())

		metricsSrv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
			Handler: mux,
		}

		go func() {
			slog.Info("Starting metrics server", "addr", metricsSrv.Addr)
			if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("Metrics Server shutting down", "error", err)
			}
		}()
	}

	fmt.Printf("Straw Proxy Relay Server %s started on %s\n", Version, srv.Address())

	// 6. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	if err := adminSrv.Stop(ctx); err != nil {
		slog.Error("Admin Server forced to shutdown", "error", err)
	}

	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(ctx); err != nil {
			slog.Error("Metrics Server forced to shutdown", "error", err)
		}
	}

	slog.Info("Server exiting")
}

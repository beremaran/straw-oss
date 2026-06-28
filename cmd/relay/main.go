package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/observability/logging"
	"github.com/beremaran/straw/internal/observability/metrics"
	"github.com/beremaran/straw/internal/observability/tracing"
	"github.com/beremaran/straw/internal/server"
	"github.com/beremaran/straw/internal/server/admin"
	"github.com/beremaran/straw/internal/service/auth"
	"github.com/beremaran/straw/internal/service/endpoint"
	"github.com/beremaran/straw/internal/service/filter"
	"github.com/beremaran/straw/internal/service/orchestrator"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/internal/service/session"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/google/uuid"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func intPtr(i int) *int {
	return &i
}

func main() {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.SetupLogger(logging.Config{
		Level:   cfg.Observability.LogLevel,
		Format:  cfg.Observability.LogFormat,
		Service: "relay",
		Version: Version,
	})
	slog.SetDefault(logger)
	ctx := context.Background()

	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-relay", Version)
	if err != nil {
		slog.Warn("Failed to initialize tracer provider", "error", err)
	} else {
		defer func() {
			err := shutdownTracer(ctx)
			if err != nil {
				slog.Error("Failed to shutdown tracer provider", "error", err)
			}
		}()
	}

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

	pgClient, err := postgres.NewClient(ctx, cfg.Database.DSN, pgBreaker)
	if err != nil {
		slog.Error("Failed to connect to Postgres", "error", err)
		os.Exit(1)
	}
	defer pgClient.Close()

	if cfg.Database.AutoMigrate {
		slog.Info("Applying pending migrations...")
		err := postgres.RunEmbeddedMigrations(cfg.Database.DSN)
		if err != nil {
			slog.Error("Failed to run migrations", "error", err)
			os.Exit(1)
		}
		slog.Info("Migrations applied successfully!")
	}

	redisClient, err := redis.NewClient(cfg.Redis, redisBreaker)
	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	apiKeyRepo := postgres.NewApiKeyRepository(pgClient)
	authCache := auth.NewAuthCache(redisClient, 5*time.Minute)
	authService := auth.NewAuthService(apiKeyRepo, authCache)

	sessionStore := session.NewRedisStore(redisClient)
	sessionService := session.NewService(sessionStore)

	ruleRepo := postgres.NewRoutingRuleRepository(pgClient)
	ruleCache := router.NewRuleCache(redisClient.Client, 10*time.Minute)
	matcher := router.NewMatcher(ruleRepo, ruleCache)

	rateLimiter := ratelimit.NewRateLimiter(redisClient)

	abpMatcher := filter.NewABPMatcher(redisClient, filter.ABPMatcherConfig{
		UpdateInterval: 24 * time.Hour,
	})

	go abpMatcher.StartAutoUpdate(ctx)

	filterService := filter.NewService(abpMatcher)

	endpointHealthStore := redis.NewEndpointHealthStore(redisClient)
	endpointSelector := orchestrator.NewSimpleEndpointSelector(endpointHealthStore)

	natsBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)
	if err := natsBroker.Connect(); err != nil {
		slog.Error("Failed to connect to message broker", "error", err)
		os.Exit(1)
	}
	defer func() { _ = natsBroker.Close() }()

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

	if err := natsBroker.DeclareQueue(ctx, "heartbeats"); err != nil {
		slog.Error("Failed to declare heartbeats queue", "error", err)
		os.Exit(1)
	}

	if err := natsBroker.BindQueue(ctx, "heartbeats", "heartbeats", ""); err != nil {
		slog.Error("Failed to bind heartbeats queue to exchange", "error", err)
		os.Exit(1)
	}
	slog.Info("NATS heartbeats consumer bound to stream")

	if err := natsBroker.DeclareQueue(ctx, orchestrator.SharedResultQueue); err != nil {
		slog.Error("Failed to declare result queue", "error", err)
		os.Exit(1)
	}

	if err := natsBroker.BindQueue(ctx, orchestrator.SharedResultQueue, "", orchestrator.SharedResultQueue); err != nil {
		slog.Error("Failed to bind result queue to exchange", "error", err)
		os.Exit(1)
	}
	slog.Info("NATS result consumer bound to stream")

	publisher := orchestrator.NewPublisher(natsBroker, endpointSelector, []byte(cfg.Security.HMACSecret), rabbitBreaker)
	consumer := orchestrator.NewConsumer(natsBroker)

	retryExecutor := orchestrator.NewRetryExecutor(
		publisher,
		consumer,
		endpointSelector,
		natsBroker,
		[]byte(cfg.Security.HMACSecret),
	)

	if err := retryExecutor.Start(ctx); err != nil {
		slog.Error("Failed to start retry executor", "error", err)
		os.Exit(1)
	}

	if err := matcher.LoadRules(ctx); err != nil {
		slog.Warn("Failed to load initial routing rules", "error", err)
	}

	matcher.StartAutoRefresh(ctx, 5*time.Second)

	if keysExist, err := apiKeyRepo.Exists(ctx); err != nil {
		slog.Warn("Failed to check for existing API keys", "error", err)
	} else if !keysExist {
		keyBytes := make([]byte, 32)
		if _, err := rand.Read(keyBytes); err != nil {
			slog.Error("Failed to generate admin key", "error", err)
			os.Exit(1)
		}
		rawKey := hex.EncodeToString(keyBytes)
		hash := sha256.Sum256([]byte(rawKey))
		tokenHash := hex.EncodeToString(hash[:])

		adminKey := domain.NewApiKey(uuid.New().String(), tokenHash, "Default Administrator Key", []string{"target:*", "type:*", "region:*"})
		adminKey.RateLimitOverride = intPtr(100)

		err := apiKeyRepo.Create(ctx, adminKey)
		if err != nil {
			slog.Error("Failed to create initial admin API key", "error", err)
			os.Exit(1)
		}

		fmt.Printf("Initial admin API key generated: %s\n", rawKey)
		fmt.Println("Save this key — it will not be shown again.")
	}

	var serverOpts []server.Option
	if cfg.AllowPrivateIPs {
		slog.Warn("Private IP validation disabled - SSRF protection bypassed (testing mode)")
		serverOpts = append(serverOpts, server.WithAllowPrivateIPs())
	}
	srv := server.New(*cfg, authService, sessionService, matcher, rateLimiter, filterService, retryExecutor, serverOpts...)

	healthService := endpoint.NewHealthService(natsBroker, endpointHealthStore)
	if err := healthService.Start(ctx); err != nil {
		slog.Warn("Failed to start health service", "error", err)
	}
	defer healthService.Stop()

	adminSrv := admin.New(*cfg, pgClient, redisClient, healthService, natsBroker)

	go func() {
		err := srv.Start()
		if err != nil {
			slog.Error("Server shutting down", "error", err)
		}
	}()

	go func() {
		err := adminSrv.Start()
		if err != nil {
			slog.Error("Admin Server shutting down", "error", err)
		}
	}()

	var metricsSrv *http.Server
	if cfg.Observability.MetricsEnabled {
		metrics.Init()
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())

		metrics.RegisterPprof(mux)

		metricsSrv = &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
			Handler: mux,
		}

		go func() {
			slog.Info("Starting metrics server", "addr", metricsSrv.Addr)
			err := metricsSrv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("Metrics Server shutting down", "error", err)
			}
		}()
	}

	fmt.Printf("Straw Proxy Relay Server %s started on %s\n", Version, srv.Address())

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
		err := metricsSrv.Shutdown(ctx)
		if err != nil {
			slog.Error("Metrics Server forced to shutdown", "error", err)
		}
	}

	slog.Info("Server exiting")
}

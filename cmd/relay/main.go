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

type relayContext struct {
	cfg              *config.ServerConfig
	ctx              context.Context
	pgClient         *postgres.Client
	redisClient      *redis.Client
	natsBroker       *broker.NatsBroker
	authService      *auth.Service
	sessionService   *session.Service
	matcher          *router.Matcher
	rateLimiter      *ratelimit.RateLimiter
	filterService    *filter.Service
	endpointSelector *orchestrator.SimpleEndpointSelector
	publisher        *orchestrator.Publisher
	consumer         *orchestrator.Consumer
	retryExecutor    *orchestrator.RetryExecutor
	srv              *server.Server
	adminSrv         *admin.Server
	healthService    *endpoint.HealthService
	apiKeyRepo       *postgres.ApiKeyRepository
}

func intPtr(i int) *int {
	return &i
}

func main() {
	_, rc, err := setupRelay()
	if err != nil {
		slog.Error("Failed to start relay", "error", err)
		os.Exit(1)
	}
	defer teardownRelay(rc)

	rc.srv.Start()
	rc.adminSrv.Start()

	fmt.Printf("Straw Proxy Relay Server %s started on %s\n", Version, rc.srv.Address())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), rc.cfg.ShutdownTimeout)
	defer cancel()

	teardownServers(rc, shutdownCtx)

	slog.Info("Server exiting")
}

func setupRelay() (context.Context, *relayContext, error) {
	ctx, cfg, err := setupConfigAndLogger()
	if err != nil {
		return nil, nil, err
	}

	pgClient, redisClient, err := setupDatabases(ctx, cfg)
	if err != nil {
		return nil, nil, err
	}

	services, err := setupServices(ctx, cfg, pgClient, redisClient)
	if err != nil {
		redisClient.Close()
		pgClient.Close()
		return nil, nil, err
	}

	natsBroker, err := setupNATSFull(ctx, cfg, redisClient, pgClient)
	if err != nil {
		return nil, nil, err
	}

	retryExecutor, err := setupOrchestrator(ctx, natsBroker, services.endpointSelector, cfg, services)
	if err != nil {
		natsBroker.Close()
		redisClient.Close()
		pgClient.Close()
		return nil, nil, err
	}

	srv, adminSrv, healthService, err := setupServers(ctx, cfg, pgClient, redisClient, natsBroker, services, retryExecutor)
	if err != nil {
		natsBroker.Close()
		redisClient.Close()
		pgClient.Close()
		return nil, nil, err
	}

	rc := &relayContext{
		cfg: cfg, ctx: ctx, pgClient: pgClient, redisClient: redisClient,
		natsBroker: natsBroker, authService: services.authService,
		sessionService: services.sessionService, matcher: services.matcher,
		rateLimiter: services.rateLimiter, filterService: services.filterService,
		endpointSelector: services.endpointSelector, publisher: services.publisher,
		consumer: services.consumer, retryExecutor: retryExecutor,
		srv: srv, adminSrv: adminSrv, healthService: healthService,
		apiKeyRepo: services.apiKeyRepo,
	}

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

	return ctx, rc, nil
}

type services struct {
	apiKeyRepo       *postgres.ApiKeyRepository
	authService      *auth.Service
	sessionService   *session.Service
	matcher          *router.Matcher
	rateLimiter      *ratelimit.RateLimiter
	filterService    *filter.Service
	endpointSelector *orchestrator.SimpleEndpointSelector
	publisher        *orchestrator.Publisher
	consumer         *orchestrator.Consumer
}

func setupConfigAndLogger() (context.Context, *config.ServerConfig, error) {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("load config: %w", err)
	}

	logging.SetupLogger(logging.Config{
		Level:   cfg.Observability.LogLevel,
		Format:  cfg.Observability.LogFormat,
		Service: "relay",
		Version: Version,
	})
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	ctx := context.Background()

	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-relay", Version)
	if err != nil {
		slog.Warn("Failed to initialize tracer provider", "error", err)
	} else {
		defer func() {
			if err := shutdownTracer(ctx); err != nil {
				slog.Error("Failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	return ctx, cfg, nil
}

func setupDatabases(ctx context.Context, cfg *config.ServerConfig) (*postgres.Client, *redis.Client, error) {
	pgBreaker := circuitbreaker.New(circuitbreaker.Config{
		Name: "postgres", FailureThreshold: 5, ResetTimeout: 30 * time.Second,
	})
	redisBreaker := circuitbreaker.New(circuitbreaker.Config{
		Name: "redis", FailureThreshold: 10, ResetTimeout: 10 * time.Second,
	})

	pgClient, err := postgres.NewClient(ctx, cfg.Database.DSN, pgBreaker)
	if err != nil {
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}

	if cfg.Database.AutoMigrate {
		slog.Info("Applying pending migrations...")
		if err := postgres.RunEmbeddedMigrations(cfg.Database.DSN); err != nil {
			pgClient.Close()
			return nil, nil, fmt.Errorf("migrations: %w", err)
		}
		slog.Info("Migrations applied successfully!")
	}

	redisClient, err := redis.NewClient(ctx, cfg.Redis, redisBreaker)
	if err != nil {
		pgClient.Close()
		return nil, nil, fmt.Errorf("connect redis: %w", err)
	}

	return pgClient, redisClient, nil
}

func setupServices(ctx context.Context, cfg *config.ServerConfig, pgClient *postgres.Client, redisClient *redis.Client) (*services, error) {
	rabbitBreaker := circuitbreaker.New(circuitbreaker.Config{
		Name: "rabbitmq", FailureThreshold: 5, ResetTimeout: 20 * time.Second,
	})

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

	publisher := orchestrator.NewPublisher(nil, endpointSelector, []byte(cfg.Security.HMACSecret), rabbitBreaker)
	consumer := orchestrator.NewConsumer(nil)

	return &services{
		apiKeyRepo: apiKeyRepo, authService: authService, sessionService: sessionService,
		matcher: matcher, rateLimiter: rateLimiter, filterService: filterService,
		endpointSelector: endpointSelector, publisher: publisher, consumer: consumer,
	}, nil
}

func setupNATSFull(ctx context.Context, cfg *config.ServerConfig, redisClient *redis.Client, pgClient *postgres.Client) (*broker.NatsBroker, error) {
	natsBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)
	if err := natsBroker.Connect(); err != nil {
		redisClient.Close()
		pgClient.Close()
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	if err := setupNATSStrams(natsBroker, ctx); err != nil {
		natsBroker.Close()
		redisClient.Close()
		pgClient.Close()
		return nil, fmt.Errorf("setup NATS streams: %w", err)
	}
	if err := setupNATSQueues(natsBroker, ctx); err != nil {
		natsBroker.Close()
		redisClient.Close()
		pgClient.Close()
		return nil, fmt.Errorf("setup NATS queues: %w", err)
	}

	return natsBroker, nil
}

func setupOrchestrator(ctx context.Context, natsBroker *broker.NatsBroker, endpointSelector *orchestrator.SimpleEndpointSelector, cfg *config.ServerConfig, svc *services) (*orchestrator.RetryExecutor, error) {
	rabbitBreaker := circuitbreaker.New(circuitbreaker.Config{
		Name: "rabbitmq", FailureThreshold: 5, ResetTimeout: 20 * time.Second,
	})

	svc.publisher = orchestrator.NewPublisher(natsBroker, endpointSelector, []byte(cfg.Security.HMACSecret), rabbitBreaker)
	svc.consumer = orchestrator.NewConsumer(natsBroker)

	retryExecutor := orchestrator.NewRetryExecutor(
		svc.publisher, svc.consumer, endpointSelector, natsBroker, []byte(cfg.Security.HMACSecret),
	)
	if err := retryExecutor.Start(ctx); err != nil {
		natsBroker.Close()
		return nil, fmt.Errorf("start retry executor: %w", err)
	}

	if err := svc.matcher.LoadRules(ctx); err != nil {
		slog.Warn("Failed to load initial routing rules", "error", err)
	}
	svc.matcher.StartAutoRefresh(ctx, 5*time.Second)

	if keysExist, err := svc.apiKeyRepo.Exists(ctx); err != nil {
		slog.Warn("Failed to check for existing API keys", "error", err)
	} else if !keysExist {
		createInitialAdminKey(ctx, svc.apiKeyRepo)
	}

	return retryExecutor, nil
}

func setupServers(ctx context.Context, cfg *config.ServerConfig, pgClient *postgres.Client, redisClient *redis.Client, natsBroker *broker.NatsBroker, svc *services, retryExecutor *orchestrator.RetryExecutor) (*server.Server, *admin.Server, *endpoint.HealthService, error) {
	var serverOpts []server.Option
	if cfg.AllowPrivateIPs {
		slog.Warn("Private IP validation disabled - SSRF protection bypassed (testing mode)")
		serverOpts = append(serverOpts, server.WithAllowPrivateIPs())
	}
	srv := server.New(*cfg, svc.authService, svc.sessionService, svc.matcher, svc.rateLimiter, svc.filterService, retryExecutor, serverOpts...)

	healthService := endpoint.NewHealthService(natsBroker, redis.NewEndpointHealthStore(redisClient))
	if err := healthService.Start(ctx); err != nil {
		slog.Warn("Failed to start health service", "error", err)
	}

	adminSrv := admin.New(ctx, *cfg, pgClient, redisClient, healthService, natsBroker)

	return srv, adminSrv, healthService, nil
}

func teardownRelay(rc *relayContext) {
	rc.healthService.Stop()
	rc.natsBroker.Close()
	rc.redisClient.Close()
	rc.pgClient.Close()
}

func teardownServers(rc *relayContext, shutdownCtx context.Context) {
	if err := rc.srv.Stop(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}
	if err := rc.adminSrv.Stop(shutdownCtx); err != nil {
		slog.Error("Admin Server forced to shutdown", "error", err)
	}
}

func setupNATSStrams(b *broker.NatsBroker, ctx context.Context) error {
	exchanges := []struct{ name, kind string }{
		{"heartbeats", "fanout"},
		{"tasks", "direct"},
		{"results", "direct"},
	}
	for _, ex := range exchanges {
		if err := b.DeclareExchange(ctx, ex.name, ex.kind); err != nil {
			slog.Error("Failed to declare "+ex.name+" exchange", "error", err)
			return err
		}
	}
	slog.Info("NATS streams declared successfully", "streams", []string{"heartbeats", "tasks", "results"})
	return nil
}

func setupNATSQueues(b *broker.NatsBroker, ctx context.Context) error {
	if err := b.DeclareQueue(ctx, "heartbeats"); err != nil {
		slog.Error("Failed to declare heartbeats queue", "error", err)
		return err
	}
	if err := b.BindQueue(ctx, "heartbeats", "heartbeats", ""); err != nil {
		slog.Error("Failed to bind heartbeats queue to exchange", "error", err)
		return err
	}
	slog.Info("NATS heartbeats consumer bound to stream")

	if err := b.DeclareQueue(ctx, orchestrator.SharedResultQueue); err != nil {
		slog.Error("Failed to declare result queue", "error", err)
		return err
	}
	if err := b.BindQueue(ctx, orchestrator.SharedResultQueue, "", orchestrator.SharedResultQueue); err != nil {
		slog.Error("Failed to bind result queue to exchange", "error", err)
		return err
	}
	slog.Info("NATS result consumer bound to stream")
	return nil
}

func createInitialAdminKey(ctx context.Context, repo *postgres.ApiKeyRepository) {
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

	if err := repo.Create(ctx, adminKey); err != nil {
		slog.Error("Failed to create initial admin API key", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Initial admin API key generated: %s\n", rawKey)
	fmt.Println("Save this key — it will not be shown again.")
}

func startMetricsServer(port int) *http.Server {
	metrics.Init()
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	metrics.RegisterPprof(mux)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	go func() {
		slog.Info("Starting metrics server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Metrics Server shutting down", "error", err)
		}
	}()

	return srv
}

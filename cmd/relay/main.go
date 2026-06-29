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
	ctx := context.Background()
	cfg := loadConfigOrDie()
	setupLogger(cfg)

	shutdownTracer := tryStartTracer(ctx)
	if shutdownTracer != nil {
		defer func() {
			err := shutdownTracer(ctx)
			if err != nil {
				slog.Error("Failed to shutdown tracer provider", "error", err)
			}
		}()
	}

	pgClient := getPostgresClientOrDie(ctx, cfg)
	defer pgClient.Close()

	redisClient := getRedisClientOrDie(ctx, cfg)
	defer func() { _ = redisClient.Close() }()

	natsBroker := getNATSConnectionOrDie(ctx, cfg)
	defer func() { _ = natsBroker.Close() }()

	handleMigrations(cfg)

	apiKeyRepo, authService := initAuthServices(pgClient, redisClient)
	sessionService := initSessionServices(redisClient)
	ruleRepo, ruleCache := initRoutingServices(pgClient, redisClient)
	rateLimiter := ratelimit.NewRateLimiter(redisClient)
	filterService := initFilterService(redisClient, ctx)
	endpointHealthStore, endpointSelector := initHealthServices(redisClient)
	retryExecutor := initRetryablePubSubOrDie(natsBroker, endpointSelector, cfg, ctx)
	matcher := initRouter(ruleRepo, ruleCache, ctx)

	relayServer := createServer(cfg, authService, sessionService, matcher, rateLimiter, filterService, retryExecutor)
	healthService := startHealthServiceOrDie(natsBroker, endpointHealthStore, ctx)
	defer healthService.Stop()
	managementServer := getManagementServer(cfg, pgClient, redisClient, healthService, natsBroker, authService)
	metricsServer := startMetricsServerIfEnabled(cfg)

	ensureManagementKey(apiKeyRepo, ctx)

	fmt.Printf("Straw Proxy Relay Server %s started on %s\n", Version, relayServer.Address())

	listenInterrupts()
	shutdown(ctx, cfg, relayServer, managementServer, metricsServer)
}

func initHealthServices(redisClient *redis.Client) (*redis.EndpointHealthStore, *orchestrator.SimpleEndpointSelector) {
	endpointHealthStore := redis.NewEndpointHealthStore(redisClient)
	endpointSelector := orchestrator.NewSimpleEndpointSelector(endpointHealthStore)

	return endpointHealthStore, endpointSelector
}

func initFilterService(redisClient *redis.Client, ctx context.Context) *filter.Service {
	abpMatcher := initAdBlockMatcher(redisClient, ctx)
	filterService := filter.NewService(abpMatcher)

	return filterService
}

func initAdBlockMatcher(redisClient *redis.Client, ctx context.Context) *filter.ABPMatcher {
	abpMatcher := filter.NewABPMatcher(redisClient, filter.ABPMatcherConfig{
		UpdateInterval: 24 * time.Hour,
	})
	go abpMatcher.StartAutoUpdate(ctx)

	return abpMatcher
}

func initRoutingServices(pgClient *postgres.Client, redisClient *redis.Client) (*postgres.RoutingRuleRepository, *router.RuleCache) {
	ruleRepo := postgres.NewRoutingRuleRepository(pgClient)
	ruleCache := router.NewRuleCache(redisClient.Client, 10*time.Minute)

	return ruleRepo, ruleCache
}

func initSessionServices(redisClient *redis.Client) *session.Service {
	sessionStore := session.NewRedisStore(redisClient)
	sessionService := session.NewService(sessionStore)

	return sessionService
}

func initAuthServices(pgClient *postgres.Client, redisClient *redis.Client) (*postgres.ApiKeyRepository, *auth.Service) {
	apiKeyRepo := postgres.NewApiKeyRepository(pgClient)
	apiKeyTokenRepo := postgres.NewApiKeyTokenRepository(pgClient)

	// Build auth service
	authCache := auth.NewAuthCache(redisClient, 10*time.Minute)
	authService := auth.NewAuthService(apiKeyRepo, apiKeyTokenRepo, authCache)

	return apiKeyRepo, authService
}

func tryStartTracer(ctx context.Context) func(context.Context) error {
	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-relay", Version)
	if err != nil {
		slog.Warn("Failed to initialize tracer provider", "error", err)
	}

	return shutdownTracer
}

func listenInterrupts() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

func shutdown(ctx context.Context, cfg *config.ServerConfig, srv *server.Server, managementSrv *admin.Server, metricsSrv *http.Server) {
	ctx, cancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancel()

	tryStopServer(srv, ctx)
	tryStopManagementServer(managementSrv, ctx)
	tryStopMetricsServer(metricsSrv, ctx)

	slog.Info("Server exiting")
}

func tryStopMetricsServer(metricsSrv *http.Server, ctx context.Context) {
	if metricsSrv != nil {
		err := metricsSrv.Shutdown(ctx)
		if err != nil {
			slog.Error("Metrics Server forced to shutdown", "error", err)
		}
	}
}

func tryStopManagementServer(managementSrv *admin.Server, ctx context.Context) {
	err := managementSrv.Stop(ctx)
	if err != nil {
		slog.Error("Management Server forced to shutdown", "error", err)
	}
}

func tryStopServer(srv *server.Server, ctx context.Context) {
	err := srv.Stop(ctx)
	if err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}
}

func startMetricsServerIfEnabled(cfg *config.ServerConfig) *http.Server {
	if !cfg.Observability.MetricsEnabled {
		return nil
	}

	var metricsSrv *http.Server
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

	return metricsSrv
}

func getManagementServer(cfg *config.ServerConfig, pgClient *postgres.Client, redisClient *redis.Client, healthService *endpoint.HealthService, natsBroker *broker.NatsBroker, authService *auth.Service) *admin.Server {
	managementSrv := admin.New(*cfg, pgClient, redisClient, healthService, natsBroker, authService)
	go func() {
		err := managementSrv.Start()
		if err != nil {
			slog.Error("Management Server shutting down", "error", err)
		}
	}()

	return managementSrv
}

func createServer(cfg *config.ServerConfig, authService *auth.Service, sessionService *session.Service, matcher *router.Matcher, rateLimiter *ratelimit.RateLimiter, filterService *filter.Service, retryExecutor *orchestrator.RetryExecutor) *server.Server {
	var serverOpts []server.Option
	if cfg.AllowPrivateIPs {
		slog.Warn("Private IP validation disabled - SSRF protection bypassed (testing mode)")
		serverOpts = append(serverOpts, server.WithAllowPrivateIPs())
	}
	srv := server.New(*cfg, authService, sessionService, matcher, rateLimiter, filterService, retryExecutor, serverOpts...)
	go func() {
		err := srv.Start()
		if err != nil {
			slog.Error("Server shutting down", "error", err)
		}
	}()

	return srv
}

func startHealthServiceOrDie(natsBroker *broker.NatsBroker, endpointHealthStore *redis.EndpointHealthStore, ctx context.Context) *endpoint.HealthService {
	healthService := endpoint.NewHealthService(natsBroker, endpointHealthStore)
	err := healthService.Start(ctx)
	if err != nil {
		slog.Warn("Failed to start health service", "error", err)
		os.Exit(1)
	}

	return healthService
}

func initRouter(ruleRepo *postgres.RoutingRuleRepository, ruleCache *router.RuleCache, ctx context.Context) *router.Matcher {
	matcher := router.NewMatcher(ruleRepo, ruleCache)
	err := matcher.LoadRules(ctx)
	if err != nil {
		slog.Warn("Failed to load initial routing rules", "error", err)
	}
	matcher.StartAutoRefresh(ctx, 5*time.Second)

	return matcher
}

func initRetryablePubSubOrDie(natsBroker *broker.NatsBroker, endpointSelector *orchestrator.SimpleEndpointSelector, cfg *config.ServerConfig, ctx context.Context) *orchestrator.RetryExecutor {
	publisher := orchestrator.NewPublisher(natsBroker, endpointSelector, []byte(cfg.Security.HMACSecret), circuitbreaker.New(circuitbreaker.Config{
		Name:             "nats",
		FailureThreshold: 5,
		ResetTimeout:     20 * time.Second,
	}))
	consumer := orchestrator.NewConsumer(natsBroker)

	retryExecutor := orchestrator.NewRetryExecutor(
		publisher,
		consumer,
		endpointSelector,
		natsBroker,
		[]byte(cfg.Security.HMACSecret),
	)

	err := retryExecutor.Start(ctx)
	if err != nil {
		slog.Error("Failed to start retry executor", "error", err)
		os.Exit(1)
	}

	return retryExecutor
}

func ensureManagementKey(apiKeyRepo *postgres.ApiKeyRepository, ctx context.Context) {
	keysExist, err := apiKeyRepo.Exists(ctx)
	if err != nil {
		slog.Warn("Failed to check for existing API keys", "error", err)
	} else if !keysExist {
		generateInitialManagementKey(apiKeyRepo, ctx)
	}
}

func generateInitialManagementKey(apiKeyRepo *postgres.ApiKeyRepository, ctx context.Context) {
	keyBytes := make([]byte, 32)
	_, err := rand.Read(keyBytes)
	if err != nil {
		slog.Error("Failed to generate management key", "error", err)
		os.Exit(1)
	}
	rawKey := hex.EncodeToString(keyBytes)
	hash := sha256.Sum256([]byte(rawKey))
	tokenHash := hex.EncodeToString(hash[:])

	managementKey := domain.NewApiKey(uuid.New().String(), tokenHash, "Default Management Key", []string{"target:*", "type:*", "region:*"})
	managementKey.RateLimitOverride = intPtr(100)

	err = apiKeyRepo.Create(ctx, managementKey)
	if err != nil {
		slog.Error("Failed to create initial management API key", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Initial management API key generated: %s\n", rawKey)
	fmt.Println("Save this key — it will not be shown again.")
}

func initialiseNATSOrDie(natsBroker *broker.NatsBroker, ctx context.Context) {
	err := natsBroker.DeclareStream(ctx, "heartbeats", "heartbeats.>")
	if err != nil {
		slog.Error("Failed to declare heartbeats stream", "error", err)
		os.Exit(1)
	}

	err = natsBroker.DeclareStream(ctx, "tasks", "tasks.>")
	if err != nil {
		slog.Error("Failed to declare tasks stream", "error", err)
		os.Exit(1)
	}

	err = natsBroker.DeclareStream(ctx, "results", "results.>")
	if err != nil {
		slog.Error("Failed to declare results stream", "error", err)
		os.Exit(1)
	}

	slog.Info("NATS streams declared successfully", "streams", []string{"heartbeats", "tasks", "results"})
}

func getNATSConnectionOrDie(ctx context.Context, cfg *config.ServerConfig) *broker.NatsBroker {
	natsBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)
	err := natsBroker.Connect()
	if err != nil {
		slog.Error("Failed to connect to message broker", "error", err)
		os.Exit(1)
	}

	initialiseNATSOrDie(natsBroker, ctx)

	return natsBroker
}

func getRedisClientOrDie(ctx context.Context, cfg *config.ServerConfig) *redis.Client {
	redisClient, err := redis.NewClient(ctx, cfg.Redis, circuitbreaker.New(circuitbreaker.Config{
		Name:             "redis",
		FailureThreshold: 10,
		ResetTimeout:     10 * time.Second,
	}))

	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}

	return redisClient
}

func getPostgresClientOrDie(ctx context.Context, cfg *config.ServerConfig) *postgres.Client {
	pgClient, err := postgres.NewClient(ctx, cfg.Database.DSN, circuitbreaker.New(circuitbreaker.Config{
		Name:             "postgres",
		FailureThreshold: 5,
		ResetTimeout:     30 * time.Second,
	}))

	if err != nil {
		slog.Error("Failed to connect to Postgres", "error", err)
		os.Exit(1)
	}

	return pgClient
}

func loadConfigOrDie() *config.ServerConfig {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	return cfg
}

func setupLogger(cfg *config.ServerConfig) {
	logger := logging.SetupLogger(logging.Config{
		Level:   cfg.Observability.LogLevel,
		Format:  cfg.Observability.LogFormat,
		Service: "relay",
		Version: Version,
	})

	slog.SetDefault(logger)
}

func handleMigrations(cfg *config.ServerConfig) {
	if !cfg.Database.AutoMigrate {
		return
	}

	slog.Info("Applying pending migrations...")
	err := postgres.RunEmbeddedMigrations(context.Background(), cfg.Database.DSN)
	if err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}

	slog.Info("Migrations applied successfully!")
}

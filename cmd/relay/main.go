// Package main implements the Straw relay server entrypoint.
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

	"github.com/google/uuid"

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
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

const (
	defaultUpdateInterval      = 24 * time.Hour
	ruleCacheTTL               = 10 * time.Minute
	authCacheTTL               = 10 * time.Minute
	autoRefreshInterval        = 5 * time.Second
	natsCBFailureThreshold     = 5
	natsCBResetTimeout         = 20 * time.Second
	keyByteSize                = 32
	managementRateLimit        = 100
	redisCBFailureThreshold    = 10
	redisCBResetTimeout        = 10 * time.Second
	postgresCBFailureThreshold = 5
	postgresCBResetTimeout     = 30 * time.Second
	metricsReadHeaderTimeout   = 30 * time.Second
)

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

	pgClient, redisClient, natsBroker := connectInfraOrDie(ctx, cfg)
	defer pgClient.Close()
	defer func() { _ = redisClient.Close() }()
	defer func() { _ = natsBroker.Close() }()

	apiKeyRepo, authService := initAuthServices(pgClient, redisClient)
	sessionService := initSessionServices(redisClient)
	ruleRepo, ruleCache := initRoutingServices(pgClient, redisClient)
	rateLimiter := ratelimit.NewRateLimiter(redisClient)
	filterService := initFilterService(ctx, redisClient)
	endpointHealthStore, endpointSelector := initHealthServices(redisClient)
	retryExecutor := initRetryablePubSubOrDie(ctx, natsBroker, endpointSelector, cfg)
	matcher := initRouter(ctx, ruleRepo, ruleCache)

	relayServer := createServer(cfg, authService, sessionService, matcher, rateLimiter, filterService, retryExecutor)

	healthService := startHealthServiceOrDie(ctx, natsBroker, endpointHealthStore)
	defer healthService.Stop()

	commandService := startCommandServiceIfEnabled(ctx, natsBroker, pgClient)
	if commandService != nil {
		defer commandService.Stop()
	}

	logService := startLogServiceIfEnabled(ctx, natsBroker, pgClient)
	if logService != nil {
		defer logService.Stop()
	}

	managementServer := getManagementServer(cfg, pgClient, redisClient, healthService, natsBroker, authService)
	metricsServer := startMetricsServerIfEnabled(cfg)

	ensureManagementKey(ctx, apiKeyRepo)

	fmt.Printf("Straw Proxy Relay Server %s started on %s\n", Version, relayServer.Address())

	listenInterrupts()
	shutdown(ctx, cfg, relayServer, managementServer, metricsServer)
}

func initHealthServices(redisClient *redis.Client) (*redis.EndpointHealthStore, *orchestrator.SimpleEndpointSelector) {
	endpointHealthStore := redis.NewEndpointHealthStore(redisClient)
	endpointSelector := orchestrator.NewSimpleEndpointSelector(endpointHealthStore)

	return endpointHealthStore, endpointSelector
}

func initFilterService(ctx context.Context, redisClient *redis.Client) *filter.Service {
	abpMatcher := initAdBlockMatcher(ctx, redisClient)
	filterService := filter.NewService(abpMatcher)

	return filterService
}

func initAdBlockMatcher(ctx context.Context, redisClient *redis.Client) *filter.ABPMatcher {
	abpMatcher := filter.NewABPMatcher(redisClient, filter.ABPMatcherConfig{
		UpdateInterval: defaultUpdateInterval,
	})
	go abpMatcher.StartAutoUpdate(ctx)

	return abpMatcher
}

func initRoutingServices(pgClient *postgres.Client, redisClient *redis.Client) (*postgres.RoutingRuleRepository, *router.RuleCache) {
	ruleRepo := postgres.NewRoutingRuleRepository(pgClient)
	ruleCache := router.NewRuleCache(redisClient.Client, ruleCacheTTL)

	return ruleRepo, ruleCache
}

func initSessionServices(redisClient *redis.Client) *session.Service {
	sessionStore := session.NewRedisStore(redisClient)
	sessionService := session.NewService(sessionStore)

	return sessionService
}

func initAuthServices(pgClient *postgres.Client, redisClient *redis.Client) (*postgres.APIKeyRepository, *auth.Service) {
	apiKeyRepo := postgres.NewAPIKeyRepository(pgClient)
	apiKeyTokenRepo := postgres.NewAPIKeyTokenRepository(pgClient)

	// Build auth service
	authCache := auth.NewAuthCache(redisClient, authCacheTTL)
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

	tryStopServer(ctx, srv)
	tryStopManagementServer(ctx, managementSrv)
	tryStopMetricsServer(ctx, metricsSrv)

	slog.Info("Server exiting")
}

func tryStopMetricsServer(ctx context.Context, metricsSrv *http.Server) {
	if metricsSrv != nil {
		err := metricsSrv.Shutdown(ctx)
		if err != nil {
			slog.Error("Metrics Server forced to shutdown", "error", err)
		}
	}
}

func tryStopManagementServer(ctx context.Context, managementSrv *admin.Server) {
	err := managementSrv.Stop(ctx)
	if err != nil {
		slog.Error("Management Server forced to shutdown", "error", err)
	}
}

func tryStopServer(ctx context.Context, srv *server.Server) {
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
		Addr:              fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
		Handler:           mux,
		ReadHeaderTimeout: metricsReadHeaderTimeout,
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

func startHealthServiceOrDie(ctx context.Context, natsBroker *broker.NatsBroker, endpointHealthStore *redis.EndpointHealthStore) *endpoint.HealthService {
	healthService := endpoint.NewHealthService(natsBroker, endpointHealthStore)

	err := healthService.Start(ctx)
	if err != nil {
		slog.Warn("Failed to start health service", "error", err)
		os.Exit(1)
	}

	return healthService
}

func initRouter(ctx context.Context, ruleRepo *postgres.RoutingRuleRepository, ruleCache *router.RuleCache) *router.Matcher {
	matcher := router.NewMatcher(ruleRepo, ruleCache)

	err := matcher.LoadRules(ctx)
	if err != nil {
		slog.Warn("Failed to load initial routing rules", "error", err)
	}

	matcher.StartAutoRefresh(ctx, autoRefreshInterval)

	return matcher
}

func initRetryablePubSubOrDie(ctx context.Context, natsBroker *broker.NatsBroker, endpointSelector *orchestrator.SimpleEndpointSelector, cfg *config.ServerConfig) *orchestrator.RetryExecutor {
	publisher := orchestrator.NewPublisher(natsBroker, endpointSelector, []byte(cfg.Security.HMACSecret), circuitbreaker.New(circuitbreaker.Config{
		Name:             "nats",
		FailureThreshold: natsCBFailureThreshold,
		ResetTimeout:     natsCBResetTimeout,
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

func ensureManagementKey(ctx context.Context, apiKeyRepo *postgres.APIKeyRepository) {
	keysExist, err := apiKeyRepo.Exists(ctx)
	if err != nil {
		slog.Warn("Failed to check for existing API keys", "error", err)
	} else if !keysExist {
		generateInitialManagementKey(ctx, apiKeyRepo)
	}
}

func generateInitialManagementKey(ctx context.Context, apiKeyRepo *postgres.APIKeyRepository) {
	keyBytes := make([]byte, keyByteSize)

	_, err := rand.Read(keyBytes)
	if err != nil {
		slog.Error("Failed to generate management key", "error", err)
		os.Exit(1)
	}

	rawKey := hex.EncodeToString(keyBytes)
	hash := sha256.Sum256([]byte(rawKey))
	tokenHash := hex.EncodeToString(hash[:])

	managementKey := domain.NewAPIKey(uuid.New().String(), tokenHash, "Default Management Key", []string{"target:*", "type:*", "region:*"})
	managementKey.RateLimitOverride = (func(i int) *int { return &i })(managementRateLimit)

	err = apiKeyRepo.Create(ctx, managementKey)
	if err != nil {
		slog.Error("Failed to create initial management API key", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Initial management API key generated: %s\n", rawKey)
	fmt.Println("Save this key — it will not be shown again.")
}

func initialiseNATSOrDie(ctx context.Context, natsBroker *broker.NatsBroker) {
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

	initialiseNATSOrDie(ctx, natsBroker)

	return natsBroker
}

func getRedisClientOrDie(ctx context.Context, cfg *config.ServerConfig) *redis.Client {
	redisClient, err := redis.NewClient(ctx, cfg.Redis, circuitbreaker.New(circuitbreaker.Config{
		Name:             "redis",
		FailureThreshold: redisCBFailureThreshold,
		ResetTimeout:     redisCBResetTimeout,
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
		FailureThreshold: postgresCBFailureThreshold,
		ResetTimeout:     postgresCBResetTimeout,
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

func handleMigrations(ctx context.Context, cfg *config.ServerConfig) {
	if !cfg.Database.AutoMigrate {
		return
	}

	slog.Info("Applying pending migrations...")

	err := postgres.RunEmbeddedMigrations(ctx, cfg.Database.DSN)
	if err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}

	slog.Info("Migrations applied successfully!")
}

func startCommandServiceIfEnabled(ctx context.Context, natsBroker *broker.NatsBroker, pgClient *postgres.Client) *endpoint.CommandService {
	if pgClient == nil {
		return nil
	}

	commandRepo := postgres.NewEndpointCommandRepository(pgClient)
	commandService := endpoint.NewCommandService(natsBroker, commandRepo, nil)

	err := commandService.Start(ctx)
	if err != nil {
		slog.Warn("Failed to start command service", "error", err)

		return nil
	}

	return commandService
}

func startLogServiceIfEnabled(ctx context.Context, natsBroker *broker.NatsBroker, pgClient *postgres.Client) *endpoint.LogService {
	if pgClient == nil {
		return nil
	}

	logRepo := postgres.NewEndpointLogRepository(pgClient)
	logService := endpoint.NewLogService(natsBroker, logRepo, nil)

	err := logService.Start(ctx)
	if err != nil {
		slog.Warn("Failed to start log service", "error", err)

		return nil
	}

	return logService
}

func connectInfraOrDie(ctx context.Context, cfg *config.ServerConfig) (*postgres.Client, *redis.Client, *broker.NatsBroker) {
	pgClient := getPostgresClientOrDie(ctx, cfg)
	redisClient := getRedisClientOrDie(ctx, cfg)
	natsBroker := getNATSConnectionOrDie(ctx, cfg)

	handleMigrations(ctx, cfg)

	return pgClient, redisClient, natsBroker
}

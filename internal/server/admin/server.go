package admin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/infra/postgres"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/server/admin/handlers"
	"github.com/kwilabs/straw-proxy-server/internal/server/admin/middleware"
	"github.com/kwilabs/straw-proxy-server/internal/service/endpoint"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/labstack/echo/v4"
	echoMiddleware "github.com/labstack/echo/v4/middleware"
)

// Server represents the Admin HTTP server.
type Server struct {
	echo          *echo.Echo
	conf          config.ServerConfig
	client        *postgres.Client
	redisClient   *redis.Client
	healthService *endpoint.HealthService
	broker        broker.MessageBroker
}

// New creates a new Admin Server instance.
func New(conf config.ServerConfig, client *postgres.Client, redisClient *redis.Client, healthService *endpoint.HealthService, broker broker.MessageBroker) *Server {
	e := echo.New()

	// Hide banner
	e.HideBanner = true
	e.HidePort = true

	// Standard Middleware
	e.Use(echoMiddleware.RequestID())
	e.Use(echoMiddleware.Logger())
	e.Use(echoMiddleware.Recover())
	e.Use(echoMiddleware.CORS())

	s := &Server{
		echo:          e,
		conf:          conf,
		client:        client,
		redisClient:   redisClient,
		healthService: healthService,
		broker:        broker,
	}

	s.registerRoutes()
	s.setupBroker()

	return s
}

// setupBroker declares necessary exchanges.
func (s *Server) setupBroker() {
	if s.broker != nil {
		// Declare fanout exchange for fingerprint broadcasts
		// We ignore error here as it will be logged by broker or fail on publish
		_ = s.broker.DeclareExchange(context.Background(), "fingerprint_broadcast", "fanout")
	}
}

// registerRoutes registers the Admin API routes.
func (s *Server) registerRoutes() {
	// Health Checks - Public
	s.echo.GET("/healthz", s.healthCheck)

	// Admin API - Protected
	adminGroup := s.echo.Group("/admin")
	adminGroup.Use(middleware.KeyAuth(s.conf))
	// AuditLog requires Execer interface, *pgxpool.Pool satisfies it.
	// AuditLog requires Execer interface, *postgres.Client (wrapped pool) satisfies it?
	// AuditLog likely expects *pgxpool.Pool or internal interface.
	// Let's pass s.client.Pool for now if AuditLog uses pool directly.
	if s.client != nil && s.client.Pool != nil {
		adminGroup.Use(middleware.AuditLog(s.client.Pool))
	}

	// Repositories
	apiKeyRepo := postgres.NewApiKeyRepository(s.client)
	routingRuleRepo := postgres.NewRoutingRuleRepository(s.client)
	fingerprintRepo := postgres.NewFingerprintRepository(s.client)
	usageRepo := postgres.NewUsageRepository(s.client)

	// Services
	var ruleCache *router.RuleCache
	if s.redisClient != nil {
		ruleCache = router.NewRuleCache(s.redisClient.Client, 10*time.Minute)
	}

	// Handlers
	apiKeyHandler := handlers.NewApiKeyHandler(apiKeyRepo)
	routingRuleHandler := handlers.NewRoutingRuleHandler(routingRuleRepo, ruleCache)
	endpointHandler := handlers.NewEndpointHandler(s.healthService)
	fingerprintHandler := handlers.NewFingerprintHandler(fingerprintRepo, s.broker)
	usageHandler := handlers.NewUsageHandler(usageRepo)

	var cacheHandler *handlers.CacheHandler
	if s.redisClient != nil {
		cacheHandler = handlers.NewCacheHandler(s.redisClient)
	}

	// API Keys Routes
	adminGroup.POST("/api-keys", apiKeyHandler.HandleCreateApiKey)
	adminGroup.GET("/api-keys", apiKeyHandler.HandleListApiKeys)
	adminGroup.DELETE("/api-keys/:id", apiKeyHandler.HandleRevokeApiKey)

	// Routing Rules Routes
	adminGroup.POST("/rules", routingRuleHandler.HandleCreateRoutingRule)
	adminGroup.GET("/rules", routingRuleHandler.HandleListRoutingRules)
	adminGroup.GET("/rules/:id", routingRuleHandler.HandleGetRoutingRule)
	adminGroup.PUT("/rules/:id", routingRuleHandler.HandleUpdateRoutingRule)
	adminGroup.DELETE("/rules/:id", routingRuleHandler.HandleDeleteRoutingRule)

	// Endpoint Routes
	adminGroup.GET("/endpoints", endpointHandler.HandleListEndpoints)
	adminGroup.POST("/endpoints/:id/drain", endpointHandler.HandleDrainEndpoint)

	// Fingerprint Routes
	adminGroup.GET("/fingerprints", fingerprintHandler.HandleListPresets)
	adminGroup.POST("/fingerprints", fingerprintHandler.HandleCreatePreset)
	adminGroup.POST("/fingerprints/broadcast", fingerprintHandler.HandleBroadcastPresets)

	// Usage Routes
	adminGroup.GET("/usage/summary", usageHandler.HandleGetUsageSummary)
	adminGroup.GET("/billing/estimate", usageHandler.HandleGetBillingEstimate)

	// Cache Routes
	if cacheHandler != nil {
		adminGroup.POST("/cache/clear", cacheHandler.HandleClearCache)
		adminGroup.GET("/cache/stats", cacheHandler.HandleGetCacheStats)
	}
}

// healthCheck returns safe 200 OK.
func (s *Server) healthCheck(c echo.Context) error {
	return c.String(http.StatusOK, "OK")
}

// Start starts the Admin HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.AdminPort)
	return s.echo.Start(addr)
}

// Stop stops the Admin HTTP server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

// Address returns the server address.
func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.AdminPort)
}

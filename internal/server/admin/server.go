package admin

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/admin/handlers"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	mw "github.com/beremaran/straw/internal/server/middleware"
	"github.com/beremaran/straw/internal/service/endpoint"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/pkg/broker"
)

type Server struct {
	mux           *http.ServeMux
	server        *http.Server
	conf          config.ServerConfig
	client        *postgres.Client
	redisClient   *redis.Client
	healthService *endpoint.HealthService
	broker        broker.MessageBroker
}

func New(conf config.ServerConfig, client *postgres.Client, redisClient *redis.Client, healthService *endpoint.HealthService, broker broker.MessageBroker) *Server {
	mux := http.NewServeMux()

	s := &Server{
		mux:           mux,
		conf:          conf,
		client:        client,
		redisClient:   redisClient,
		healthService: healthService,
		broker:        broker,
	}

	s.registerRoutes()
	s.setupBroker()

	handler := applyMiddlewares(mux,
		mw.Recover(),
		mw.RequestID(),
		mw.LoggerMiddleware(),
		mw.CORS(),
		managementGlobalMiddleware(conf, client),
	)

	s.server = &http.Server{
		Handler: handler,
	}

	return s
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.ManagementPort)
	s.server.Addr = addr

	return s.server.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.ManagementPort)
}

func (s *Server) GetHandler() http.Handler {
	return s.server.Handler
}

func (s *Server) setupBroker() {
	if s.broker != nil {
		_ = s.broker.DeclareStream(context.Background(), "fingerprint_broadcast", "fingerprint_broadcast")
	}
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", s.healthCheck)

	apiKeyRepo := postgres.NewApiKeyRepository(s.client)
	routingRuleRepo := postgres.NewRoutingRuleRepository(s.client)
	fingerprintRepo := postgres.NewFingerprintRepository(s.client)
	usageRepo := postgres.NewUsageRepository(s.client)

	var ruleCache *router.RuleCache
	if s.redisClient != nil {
		ruleCache = router.NewRuleCache(s.redisClient.Client, 10*time.Minute)
	}

	apiKeyHandler := handlers.NewApiKeyHandler(apiKeyRepo)
	routingRuleHandler := handlers.NewRoutingRuleHandler(routingRuleRepo, ruleCache)
	endpointHandler := handlers.NewEndpointHandler(s.healthService)
	fingerprintHandler := handlers.NewFingerprintHandler(fingerprintRepo, s.broker)
	usageHandler := handlers.NewUsageHandler(usageRepo)

	var cacheHandler *handlers.CacheHandler
	if s.redisClient != nil {
		cacheHandler = handlers.NewCacheHandler(s.redisClient)
	}

	s.management("POST /management/api-keys", middleware.PermissionAPIKeysWrite, apiKeyHandler.HandleCreateApiKey)
	s.management("GET /management/api-keys", middleware.PermissionAPIKeysRead, apiKeyHandler.HandleListApiKeys)
	s.management("DELETE /management/api-keys/{id}", middleware.PermissionAPIKeysRevoke, apiKeyHandler.HandleRevokeApiKey)

	s.management("POST /management/rules", middleware.PermissionRoutingRulesWrite, routingRuleHandler.HandleCreateRoutingRule)
	s.management("GET /management/rules", middleware.PermissionRoutingRulesRead, routingRuleHandler.HandleListRoutingRules)
	s.management("GET /management/rules/{id}", middleware.PermissionRoutingRulesRead, routingRuleHandler.HandleGetRoutingRule)
	s.management("PUT /management/rules/{id}", middleware.PermissionRoutingRulesWrite, routingRuleHandler.HandleUpdateRoutingRule)
	s.management("DELETE /management/rules/{id}", middleware.PermissionRoutingRulesWrite, routingRuleHandler.HandleDeleteRoutingRule)

	s.management("GET /management/endpoints", middleware.PermissionEndpointsRead, endpointHandler.HandleListEndpoints)
	s.management("POST /management/endpoints/{id}/drain", middleware.PermissionEndpointsControl, endpointHandler.HandleDrainEndpoint)

	s.management("GET /management/fingerprints", middleware.PermissionFingerprintsRead, fingerprintHandler.HandleListPresets)
	s.management("POST /management/fingerprints", middleware.PermissionFingerprintsWrite, fingerprintHandler.HandleCreatePreset)
	s.management("POST /management/fingerprints/broadcast", middleware.PermissionFingerprintsBroadcast, fingerprintHandler.HandleBroadcastPresets)

	s.management("GET /management/usage/summary", middleware.PermissionUsageRead, usageHandler.HandleGetUsageSummary)
	s.management("GET /management/billing/estimate", middleware.PermissionBillingRead, usageHandler.HandleGetBillingEstimate)

	if cacheHandler != nil {
		s.management("POST /management/cache/clear", middleware.PermissionCacheWrite, cacheHandler.HandleClearCache)
		s.management("GET /management/cache/stats", middleware.PermissionCacheRead, cacheHandler.HandleGetCacheStats)
	}
}

func (s *Server) management(pattern, permission string, handler http.HandlerFunc) {
	s.mux.Handle(pattern, middleware.RequirePermission(permission)(handler))
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func applyMiddlewares(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}

func managementGlobalMiddleware(cfg config.ServerConfig, client *postgres.Client) func(http.Handler) http.Handler {
	keyAuth := middleware.KeyAuth(cfg)
	var auditLog func(http.Handler) http.Handler
	if client != nil && client.Pool != nil {
		auditLogger := middleware.NewAuditLogger(client.Pool, 0, 0)
		auditLog = middleware.AuditLog(auditLogger)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/management") {
				inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					next.ServeHTTP(w, r)
				})

				var h http.Handler = inner
				if auditLog != nil {
					h = auditLog(h)
				}
				h = keyAuth(h)
				h.ServeHTTP(w, r)

				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

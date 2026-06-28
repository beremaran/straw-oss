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
		adminGlobalMiddleware(conf, client),
	)

	s.server = &http.Server{
		Handler: handler,
	}

	return s
}

func (s *Server) setupBroker() {
	if s.broker != nil {
		_ = s.broker.DeclareExchange(context.Background(), "fingerprint_broadcast", "fanout")
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

	s.mux.HandleFunc("POST /admin/api-keys", apiKeyHandler.HandleCreateApiKey)
	s.mux.HandleFunc("GET /admin/api-keys", apiKeyHandler.HandleListApiKeys)
	s.mux.HandleFunc("DELETE /admin/api-keys/{id}", apiKeyHandler.HandleRevokeApiKey)

	s.mux.HandleFunc("POST /admin/rules", routingRuleHandler.HandleCreateRoutingRule)
	s.mux.HandleFunc("GET /admin/rules", routingRuleHandler.HandleListRoutingRules)
	s.mux.HandleFunc("GET /admin/rules/{id}", routingRuleHandler.HandleGetRoutingRule)
	s.mux.HandleFunc("PUT /admin/rules/{id}", routingRuleHandler.HandleUpdateRoutingRule)
	s.mux.HandleFunc("DELETE /admin/rules/{id}", routingRuleHandler.HandleDeleteRoutingRule)

	s.mux.HandleFunc("GET /admin/endpoints", endpointHandler.HandleListEndpoints)
	s.mux.HandleFunc("POST /admin/endpoints/{id}/drain", endpointHandler.HandleDrainEndpoint)

	s.mux.HandleFunc("GET /admin/fingerprints", fingerprintHandler.HandleListPresets)
	s.mux.HandleFunc("POST /admin/fingerprints", fingerprintHandler.HandleCreatePreset)
	s.mux.HandleFunc("POST /admin/fingerprints/broadcast", fingerprintHandler.HandleBroadcastPresets)

	s.mux.HandleFunc("GET /admin/usage/summary", usageHandler.HandleGetUsageSummary)
	s.mux.HandleFunc("GET /admin/billing/estimate", usageHandler.HandleGetBillingEstimate)

	if cacheHandler != nil {
		s.mux.HandleFunc("POST /admin/cache/clear", cacheHandler.HandleClearCache)
		s.mux.HandleFunc("GET /admin/cache/stats", cacheHandler.HandleGetCacheStats)
	}
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.AdminPort)
	s.server.Addr = addr
	return s.server.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.AdminPort)
}

func (s *Server) GetHandler() http.Handler {
	return s.server.Handler
}

func applyMiddlewares(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

func adminGlobalMiddleware(cfg config.ServerConfig, client *postgres.Client) func(http.Handler) http.Handler {
	keyAuth := middleware.KeyAuth(cfg)
	var auditLog func(http.Handler) http.Handler
	if client != nil && client.Pool != nil {
		auditLogger := middleware.NewAuditLogger(client.Pool, 0, 0)
		auditLog = middleware.AuditLog(auditLogger)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/admin") {
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

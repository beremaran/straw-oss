package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/admin/handlers"
	"github.com/beremaran/straw/internal/server/admin/middleware"
	mw "github.com/beremaran/straw/internal/server/middleware"
	adminauth "github.com/beremaran/straw/internal/service/auth"
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
	authService   *adminauth.AdminService
}

func New(conf config.ServerConfig, client *postgres.Client, redisClient *redis.Client, healthService *endpoint.HealthService, broker broker.MessageBroker) *Server {
	mux := http.NewServeMux()
	var authService *adminauth.AdminService
	if client != nil {
		authService = adminauth.NewAdminService(
			postgres.NewIdentityRepository(client),
			conf.Security.HMACSecret,
			conf.ManagementAccessTokenTTL,
			conf.ManagementRefreshTokenTTL,
		)
	}

	s := &Server{
		mux:           mux,
		conf:          conf,
		client:        client,
		redisClient:   redisClient,
		healthService: healthService,
		broker:        broker,
		authService:   authService,
	}

	s.registerRoutes()
	s.setupBroker()

	handler := applyMiddlewares(mux,
		mw.Recover(),
		mw.RequestID(),
		mw.LoggerMiddleware(),
		mw.CORS(),
		managementGlobalMiddleware(conf, client, authService),
	)

	s.server = &http.Server{
		Handler: handler,
	}

	return s
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.ManagementPort)
	s.server.Addr = addr
	s.warnNoActiveOwner()

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
	var auditRepo domain.ManagementAuditRepository
	if s.client != nil {
		auditRepo = postgres.NewManagementAuditRepository(s.client.Pool)
	}

	var ruleCache *router.RuleCache
	if s.redisClient != nil {
		ruleCache = router.NewRuleCache(s.redisClient.Client, 10*time.Minute)
	}

	apiKeyHandler := handlers.NewApiKeyHandler(apiKeyRepo, auditRepo)
	routingRuleHandler := handlers.NewRoutingRuleHandler(routingRuleRepo, ruleCache, auditRepo)
	endpointHandler := handlers.NewEndpointHandler(s.healthService, auditRepo)
	fingerprintHandler := handlers.NewFingerprintHandler(fingerprintRepo, s.broker, auditRepo)
	usageHandler := handlers.NewUsageHandler(usageRepo)
	authHandler := handlers.NewAuthHandler(s.authService)

	identityRepo := postgres.NewIdentityRepository(s.client)
	userHandler := handlers.NewUserHandler(identityRepo)
	roleHandler := handlers.NewRoleHandler(identityRepo)
	idpHandler := handlers.NewIdentityProviderHandler(identityRepo)

	var cacheHandler *handlers.CacheHandler
	if s.redisClient != nil {
		cacheHandler = handlers.NewCacheHandler(s.redisClient, auditRepo)
	}

	s.management("GET /management/users", middleware.PermissionUsersRead, userHandler.HandleListUsers)
	s.management("POST /management/users", middleware.PermissionUsersWrite, userHandler.HandleCreateUser)
	s.management("GET /management/users/{id}", middleware.PermissionUsersRead, userHandler.HandleGetUser)
	s.management("PATCH /management/users/{id}", middleware.PermissionUsersWrite, userHandler.HandleUpdateUser)
	s.management("DELETE /management/users/{id}", middleware.PermissionUsersWrite, userHandler.HandleDeactivateUser)

	s.management("GET /management/roles", middleware.PermissionUsersRead, roleHandler.HandleListRoles)
	s.management("POST /management/roles", middleware.PermissionUsersWrite, roleHandler.HandleCreateRole)
	s.management("PATCH /management/roles/{id}", middleware.PermissionUsersWrite, roleHandler.HandleUpdateRole)
	s.management("DELETE /management/roles/{id}", middleware.PermissionUsersWrite, roleHandler.HandleDeleteRole)

	s.management("GET /management/identity-providers", middleware.PermissionUsersRead, idpHandler.HandleListIdentityProviders)
	s.management("POST /management/identity-providers", middleware.PermissionUsersWrite, idpHandler.HandleCreateIdentityProvider)
	s.management("PATCH /management/identity-providers/{id}", middleware.PermissionUsersWrite, idpHandler.HandleUpdateIdentityProvider)
	s.management("DELETE /management/identity-providers/{id}", middleware.PermissionUsersWrite, idpHandler.HandleDeleteIdentityProvider)

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

	if s.authService != nil {
		s.mux.HandleFunc("POST /management/auth/login", authHandler.HandleLogin)
		s.mux.HandleFunc("POST /management/auth/refresh", authHandler.HandleRefresh)
		s.mux.HandleFunc("POST /management/auth/logout", authHandler.HandleLogout)
		s.mux.HandleFunc("GET /management/auth/me", authHandler.HandleMe)
		s.mux.HandleFunc("POST /management/users/bootstrap", authHandler.HandleBootstrapOwner)
		s.mux.HandleFunc("GET /management/auth/sso/{provider}/start", authHandler.HandleSSOStart)
		s.mux.HandleFunc("GET /management/auth/sso/{provider}/callback", authHandler.HandleSSOCallback)
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

func managementGlobalMiddleware(cfg config.ServerConfig, client *postgres.Client, verifier ...middleware.AccessTokenVerifier) func(http.Handler) http.Handler {
	keyAuth := middleware.KeyAuth(cfg)
	var accessVerifier middleware.AccessTokenVerifier
	if len(verifier) > 0 {
		accessVerifier = verifier[0]
	}
	sessionAuth := middleware.SessionAuth(accessVerifier)
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
				if !isPublicAuthRoute(r) {
					h = keyAuth(h)
					h = sessionAuth(h)
				}
				h.ServeHTTP(w, r)

				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isPublicAuthRoute(r *http.Request) bool {
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/management/auth/sso/") {
		return true
	}
	if r.Method != http.MethodPost {
		return false
	}

	return r.URL.Path == "/management/auth/login" || r.URL.Path == "/management/auth/refresh"
}

func (s *Server) warnNoActiveOwner() {
	if s.client == nil || s.client.Pool == nil {
		return
	}

	exists, err := postgres.NewIdentityRepository(s.client).ActiveOwnerExists(context.Background())
	if err != nil {
		slog.Warn("failed to check management owner", "error", err)

		return
	}
	if !exists {
		slog.Warn("no active management owner exists; bootstrap one at /management/users/bootstrap")
	}
}

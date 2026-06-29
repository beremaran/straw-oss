package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
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

const (
	ruleCacheTTL      = 10 * time.Minute
	readHeaderTimeout = 15 * time.Second
)

// Server is the management HTTP server.
type Server struct {
	mux           *http.ServeMux
	server        *http.Server
	conf          config.ServerConfig
	client        *postgres.Client
	redisClient   *redis.Client
	healthService *endpoint.HealthService
	broker        broker.MessageBroker
	authService   *adminauth.AdminService
	apiKeyAuth    *adminauth.Service
}

// New creates a new management server.
func New(conf config.ServerConfig, client *postgres.Client, redisClient *redis.Client, healthService *endpoint.HealthService, broker broker.MessageBroker, apiKeyAuth *adminauth.Service) *Server {
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
		apiKeyAuth:    apiKeyAuth,
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
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return s
}

// Start starts the management server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.ManagementPort)
	s.server.Addr = addr
	s.warnNoActiveOwner()

	return fmt.Errorf("listen: %w", s.server.ListenAndServe())
}

// Stop gracefully stops the management server.
func (s *Server) Stop(ctx context.Context) error {
	return fmt.Errorf("shutdown: %w", s.server.Shutdown(ctx))
}

// Address returns the address the server is listening on.
func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.ManagementPort)
}

// GetHandler returns the HTTP handler for the management server.
func (s *Server) GetHandler() http.Handler {
	return s.server.Handler
}

func (s *Server) setupBroker() {
	if s.broker != nil {
		_ = s.broker.DeclareStream(context.Background(), "fingerprint_broadcast", "fingerprint_broadcast")
		_ = s.broker.DeclareStream(context.Background(), "endpoint_control", "endpoint.control.>", "endpoint.logs.>")
	}
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", s.healthCheck)

	s.registerAllSubRoutes()
}

func (s *Server) registerAllSubRoutes() {
	apiKeyRepo := postgres.NewAPIKeyRepository(s.client)
	apiKeyTokenRepo := postgres.NewAPIKeyTokenRepository(s.client)
	routingRuleRepo := postgres.NewRoutingRuleRepository(s.client)
	fingerprintRepo := postgres.NewFingerprintRepository(s.client)
	identityRepo := postgres.NewIdentityRepository(s.client)
	usageRepo := postgres.NewUsageRepository(s.client)
	costMultiplierRepo := postgres.NewCostMultiplierRepository(s.client)

	var auditRepo domain.ManagementAuditRepository
	if s.client != nil {
		auditRepo = postgres.NewManagementAuditRepository(s.client.Pool)
	}

	var ruleCache *router.RuleCache
	if s.redisClient != nil {
		ruleCache = router.NewRuleCache(s.redisClient.Client, ruleCacheTTL)
	}

	var (
		endpointRepo domain.EndpointRepository
		commandRepo  domain.EndpointCommandRepository
		logRepo      domain.EndpointLogRepository
	)

	if s.client != nil {
		endpointRepo = postgres.NewEndpointRepository(s.client)
		commandRepo = postgres.NewEndpointCommandRepository(s.client)
		logRepo = postgres.NewEndpointLogRepository(s.client)
	}

	s.registerIdentityAndAuthRoutes(
		handlers.NewUserHandler(identityRepo),
		handlers.NewRoleHandler(identityRepo),
		handlers.NewIdentityProviderHandler(identityRepo),
		handlers.NewAuthHandler(s.authService),
	)
	s.registerAPIKeyRoutes(handlers.NewAPIKeyHandler(apiKeyRepo, apiKeyTokenRepo, auditRepo, s.apiKeyAuth))
	s.registerRoutingRuleRoutes(handlers.NewRoutingRuleHandler(routingRuleRepo, ruleCache, auditRepo))
	s.registerEndpointRoutes(handlers.NewEndpointHandler(s.healthService, endpointRepo, commandRepo, s.broker, auditRepo, logRepo))
	s.registerFingerprintRoutes(handlers.NewFingerprintHandler(fingerprintRepo, routingRuleRepo, identityRepo, s.broker, auditRepo))
	s.registerUsageRoutes(handlers.NewUsageHandler(usageRepo))
	s.registerCostMultiplierRoutes(handlers.NewCostMultiplierHandler(costMultiplierRepo, auditRepo))

	if s.redisClient != nil {
		s.registerCacheRoutes(handlers.NewCacheHandler(s.redisClient, auditRepo))
	}

	s.registerAuditRoutes(handlers.NewAuditHandler(auditRepo, identityRepo))
}

func (s *Server) registerIdentityAndAuthRoutes(user *handlers.UserHandler, role *handlers.RoleHandler, idp *handlers.IdentityProviderHandler, auth *handlers.AuthHandler) {
	s.registerUserRoutes(user)
	s.registerRoleRoutes(role)
	s.registerIdentityProviderRoutes(idp)
	s.registerAuthRoutes(auth)
}

func (s *Server) registerAuthRoutes(authHandler *handlers.AuthHandler) {
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

func (s *Server) registerUserRoutes(userHandler *handlers.UserHandler) {
	s.management("GET /management/users", middleware.PermissionUsersRead, userHandler.HandleListUsers)
	s.management("POST /management/users", middleware.PermissionUsersWrite, userHandler.HandleCreateUser)
	s.management("GET /management/users/{id}", middleware.PermissionUsersRead, userHandler.HandleGetUser)
	s.management("PATCH /management/users/{id}", middleware.PermissionUsersWrite, userHandler.HandleUpdateUser)
	s.management("DELETE /management/users/{id}", middleware.PermissionUsersWrite, userHandler.HandleDeactivateUser)
}

func (s *Server) registerRoleRoutes(roleHandler *handlers.RoleHandler) {
	s.management("GET /management/roles", middleware.PermissionUsersRead, roleHandler.HandleListRoles)
	s.management("POST /management/roles", middleware.PermissionUsersWrite, roleHandler.HandleCreateRole)
	s.management("PATCH /management/roles/{id}", middleware.PermissionUsersWrite, roleHandler.HandleUpdateRole)
	s.management("DELETE /management/roles/{id}", middleware.PermissionUsersWrite, roleHandler.HandleDeleteRole)
}

func (s *Server) registerIdentityProviderRoutes(idpHandler *handlers.IdentityProviderHandler) {
	s.management("GET /management/identity-providers", middleware.PermissionUsersRead, idpHandler.HandleListIdentityProviders)
	s.management("POST /management/identity-providers", middleware.PermissionUsersWrite, idpHandler.HandleCreateIdentityProvider)
	s.management("PATCH /management/identity-providers/{id}", middleware.PermissionUsersWrite, idpHandler.HandleUpdateIdentityProvider)
	s.management("DELETE /management/identity-providers/{id}", middleware.PermissionUsersWrite, idpHandler.HandleDeleteIdentityProvider)
}

func (s *Server) registerAPIKeyRoutes(apiKeyHandler *handlers.APIKeyHandler) {
	s.management("POST /management/api-keys", middleware.PermissionAPIKeysWrite, apiKeyHandler.HandleCreateAPIKey)
	s.management("GET /management/api-keys", middleware.PermissionAPIKeysRead, apiKeyHandler.HandleListAPIKeys)
	s.management("GET /management/api-keys/{id}", middleware.PermissionAPIKeysRead, apiKeyHandler.HandleGetAPIKey)
	s.management("PATCH /management/api-keys/{id}", middleware.PermissionAPIKeysWrite, apiKeyHandler.HandleUpdateAPIKey)
	s.management("POST /management/api-keys/{id}/rotate", middleware.PermissionAPIKeysRotate, apiKeyHandler.HandleRotateAPIKey)
	s.management("POST /management/api-keys/{id}/reactivate", middleware.PermissionAPIKeysWrite, apiKeyHandler.HandleReactivateAPIKey)
	s.management("POST /management/api-keys/{id}/revoke", middleware.PermissionAPIKeysRevoke, apiKeyHandler.HandleRevokeAPIKey)
	s.management("DELETE /management/api-keys/{id}", middleware.PermissionAPIKeysRevoke, apiKeyHandler.HandleRevokeAPIKey)
}

func (s *Server) registerRoutingRuleRoutes(routingRuleHandler *handlers.RoutingRuleHandler) {
	s.management("POST /management/rules", middleware.PermissionRoutingRulesWrite, routingRuleHandler.HandleCreateRoutingRule)
	s.management("GET /management/rules", middleware.PermissionRoutingRulesRead, routingRuleHandler.HandleListRoutingRules)
	s.management("GET /management/rules/{id}", middleware.PermissionRoutingRulesRead, routingRuleHandler.HandleGetRoutingRule)
	s.management("PUT /management/rules/{id}", middleware.PermissionRoutingRulesWrite, routingRuleHandler.HandleUpdateRoutingRule)
	s.management("DELETE /management/rules/{id}", middleware.PermissionRoutingRulesWrite, routingRuleHandler.HandleDeleteRoutingRule)
}

func (s *Server) registerEndpointRoutes(endpointHandler *handlers.EndpointHandler) {
	s.management("GET /management/endpoints", middleware.PermissionEndpointsRead, endpointHandler.HandleListEndpoints)
	s.management("POST /management/endpoints", middleware.PermissionEndpointsWrite, endpointHandler.HandleCreateEndpoint)
	s.management("GET /management/endpoints/{id}", middleware.PermissionEndpointsRead, endpointHandler.HandleGetEndpoint)
	s.management("PATCH /management/endpoints/{id}", middleware.PermissionEndpointsWrite, endpointHandler.HandlePatchEndpoint)
	s.management("DELETE /management/endpoints/{id}", middleware.PermissionEndpointsWrite, endpointHandler.HandleDeleteEndpoint)
	s.management("POST /management/endpoints/{id}/drain", middleware.PermissionEndpointsControl, endpointHandler.HandleDrainEndpoint)
	s.management("POST /management/endpoints/{id}/undrain", middleware.PermissionEndpointsControl, endpointHandler.HandleUndrainEndpoint)
	s.management("POST /management/endpoints/{id}/restart", middleware.PermissionEndpointsControl, endpointHandler.HandleRestartEndpoint)
	s.management("GET /management/endpoints/{id}/logs", middleware.PermissionEndpointsLogs, endpointHandler.HandleGetEndpointLogs)
	s.management("GET /management/endpoints/{id}/logs/stream", middleware.PermissionEndpointsLogs, endpointHandler.HandleStreamEndpointLogs)
	s.management("GET /management/endpoints/{id}/commands", middleware.PermissionEndpointsRead, endpointHandler.HandleListEndpointCommands)
	s.management("GET /management/commands/{id}", middleware.PermissionEndpointsRead, endpointHandler.HandleGetEndpointCommand)
}

func (s *Server) registerFingerprintRoutes(fingerprintHandler *handlers.FingerprintHandler) {
	s.management("GET /management/fingerprints", middleware.PermissionFingerprintsRead, fingerprintHandler.HandleListPresets)
	s.management("POST /management/fingerprints", middleware.PermissionFingerprintsWrite, fingerprintHandler.HandleCreatePreset)
	s.management("GET /management/fingerprints/{id}", middleware.PermissionFingerprintsRead, fingerprintHandler.HandleGetPreset)
	s.management("DELETE /management/fingerprints/{id}", middleware.PermissionFingerprintsDelete, fingerprintHandler.HandleDeletePreset)
	s.management("POST /management/fingerprints/broadcast", middleware.PermissionFingerprintsBroadcast, fingerprintHandler.HandleBroadcastPresets)
}

func (s *Server) registerUsageRoutes(usageHandler *handlers.UsageHandler) {
	s.management("GET /management/usage/summary", middleware.PermissionUsageRead, usageHandler.HandleGetUsageSummary)
	s.management("GET /management/billing/estimate", middleware.PermissionBillingRead, usageHandler.HandleGetBillingEstimate)
}

func (s *Server) registerCostMultiplierRoutes(costMultiplierHandler *handlers.CostMultiplierHandler) {
	s.management("GET /management/cost-multipliers", middleware.PermissionCostMultipliersRead, costMultiplierHandler.HandleListCostMultipliers)
	s.management("POST /management/cost-multipliers", middleware.PermissionCostMultipliersWrite, costMultiplierHandler.HandleCreateCostMultiplier)
	s.management("GET /management/cost-multipliers/{id}", middleware.PermissionCostMultipliersRead, costMultiplierHandler.HandleGetCostMultiplier)
	s.management("PUT /management/cost-multipliers/{id}", middleware.PermissionCostMultipliersWrite, costMultiplierHandler.HandleUpdateCostMultiplier)
	s.management("DELETE /management/cost-multipliers/{id}", middleware.PermissionCostMultipliersWrite, costMultiplierHandler.HandleDeleteCostMultiplier)
}

func (s *Server) registerCacheRoutes(cacheHandler *handlers.CacheHandler) {
	s.management("POST /management/cache/clear", middleware.PermissionCacheWrite, cacheHandler.HandleClearCache)
	s.management("GET /management/cache/stats", middleware.PermissionCacheRead, cacheHandler.HandleGetCacheStats)
}

func (s *Server) registerAuditRoutes(auditHandler *handlers.AuditHandler) {
	s.management("GET /management/audit/events", middleware.PermissionAuditRead, auditHandler.HandleListEvents)
	s.management("GET /management/audit/events/{id}", middleware.PermissionAuditRead, auditHandler.HandleGetEvent)
	s.management("GET /management/audit/requests", middleware.PermissionAuditRead, auditHandler.HandleListRequests)
	s.management("GET /management/audit/export", middleware.PermissionAuditRead, auditHandler.HandleExport)
}

func (s *Server) management(pattern, permission string, handler http.HandlerFunc) {
	s.mux.Handle(pattern, middleware.RequirePermission(permission)(handler))
}

func (s *Server) healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func applyMiddlewares(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for _, v := range slices.Backward(middlewares) {
		handler = v(handler)
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

package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/server/handlers"
	"github.com/kwilabs/straw-proxy-server/internal/server/metrics"
	mw "github.com/kwilabs/straw-proxy-server/internal/server/middleware"
	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/kwilabs/straw-proxy-server/internal/service/filter"
	"github.com/kwilabs/straw-proxy-server/internal/service/orchestrator"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"

	// Swagger docs import
	_ "github.com/kwilabs/straw-proxy-server/docs/relay"
)

// Server represents the HTTP server.
type Server struct {
	echo           *echo.Echo
	conf           config.ServerConfig
	authService    *auth.Service
	sessionService *session.Service
	matcher        *router.Matcher
	rateLimiter    *ratelimit.RateLimiter
	filterService  *filter.Service
	orchestrator   *orchestrator.RetryExecutor

	// Testing options
	allowPrivateIPs bool // Allow localhost/private IPs (for testing only)
}

// Option is a functional option for configuring the Server.
type Option func(*Server)

// WithAllowPrivateIPs allows URLs that resolve to private IPs.
// WARNING: Only use for testing. This disables SSRF protection.
func WithAllowPrivateIPs() Option {
	return func(s *Server) {
		s.allowPrivateIPs = true
	}
}

// New creates a new Server instance.
func New(
	conf config.ServerConfig,
	authService *auth.Service,
	sessionService *session.Service,
	matcher *router.Matcher,
	rateLimiter *ratelimit.RateLimiter,
	filterService *filter.Service,
	orchestrator *orchestrator.RetryExecutor,
	opts ...Option,
) *Server {
	e := echo.New()

	// Hide banner
	e.HideBanner = true
	e.HidePort = true

	// Initialize metrics
	metrics.Init()

	// Standard Middleware
	e.Use(middleware.RequestID())
	e.Use(mw.TracingMiddleware("straw-proxy-server"))
	e.Use(mw.MetricsMiddleware())
	e.Use(mw.LoggerMiddleware())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(middleware.BodyLimit(conf.MaxBodySize))

	s := &Server{
		echo:           e,
		conf:           conf,
		authService:    authService,
		sessionService: sessionService,
		matcher:        matcher,
		rateLimiter:    rateLimiter,
		filterService:  filterService,
		orchestrator:   orchestrator,
	}

	// Apply options
	for _, opt := range opts {
		opt(s)
	}

	s.registerRoutes()

	return s
}

// registerRoutes registers the API routes.
func (s *Server) registerRoutes() {
	// Health Checks
	s.echo.GET("/healthz", s.healthCheck)
	s.echo.GET("/readyz", s.readyCheck)

	// Swagger UI
	s.echo.GET("/swagger/*", echoSwagger.EchoWrapHandler(echoSwagger.InstanceName("relay")))

	// API Groups
	// Apply Auth Middleware to protected routes
	authMiddleware := mw.AuthMiddleware(s.authService)

	v1 := s.echo.Group("/v1")
	v1.Use(authMiddleware)
	v1.Use(mw.SessionMiddleware(s.sessionService))

	// Relay Handler configuration
	var relayOpts []handlers.RelayHandlerOption
	if s.allowPrivateIPs {
		relayOpts = append(relayOpts, handlers.WithAllowPrivateIPs())
	}

	relayHandler := handlers.NewRelayHandler(
		s.matcher,
		s.filterService,
		s.orchestrator,
		s.rateLimiter,
		s.sessionService,
		relayOpts...,
	)

	v1.POST("/request", relayHandler.Handle)

	v2 := s.echo.Group("/v2")
	v2.Use(authMiddleware)
	v2.Use(mw.SessionMiddleware(s.sessionService))

	// V2 Relay Handler (same for now)
	v2.POST("/request", relayHandler.Handle)
}

// healthCheck returns safe 200 OK.
//
//	@Summary		Health Check
//	@Description	Returns OK if the server is running
//	@Tags			health
//	@Produce		plain
//	@Success		200	{string}	string	"OK"
//	@Router			/healthz [get]
func (s *Server) healthCheck(c echo.Context) error {
	return c.String(http.StatusOK, "OK")
}

// readyCheck returns safe 200 OK.
//
//	@Summary		Readiness Check
//	@Description	Returns OK if the server is ready to accept traffic
//	@Tags			health
//	@Produce		plain
//	@Success		200	{string}	string	"OK"
//	@Router			/readyz [get]
func (s *Server) readyCheck(c echo.Context) error {
	return c.String(http.StatusOK, "OK")
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.HTTPPort)
	if s.conf.Security.TLSCertFile != "" && s.conf.Security.TLSKeyFile != "" {
		return s.echo.StartTLS(addr, s.conf.Security.TLSCertFile, s.conf.Security.TLSKeyFile)
	}
	return s.echo.Start(addr)
}

// Stop stops the HTTP server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}

// Address returns the server address.
func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.HTTPPort)
}

// GetMatcher returns the router matcher for testing purposes.
func (s *Server) GetMatcher() *router.Matcher {
	return s.matcher
}

// GetEcho returns the underlying Echo instance for testing purposes.
func (s *Server) GetEcho() *echo.Echo {
	return s.echo
}

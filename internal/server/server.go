// Package server provides the HTTP server for the Straw relay application.
package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server/handlers"
	"github.com/beremaran/straw/internal/server/metrics"
	mw "github.com/beremaran/straw/internal/server/middleware"
	"github.com/beremaran/straw/internal/service/auth"
	"github.com/beremaran/straw/internal/service/filter"
	"github.com/beremaran/straw/internal/service/orchestrator"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/internal/service/session"
)

// Server is the HTTP server for the Straw relay application.
type Server struct {
	mux             *http.ServeMux
	server          *http.Server
	conf            config.ServerConfig
	authService     *auth.Service
	sessionService  *session.Service
	matcher         *router.Matcher
	rateLimiter     *ratelimit.RateLimiter
	filterService   *filter.Service
	orchestrator    *orchestrator.RetryExecutor
	allowPrivateIPs bool
}

// Option configures a Server.
type Option func(*Server)

// WithAllowPrivateIPs allows the server to forward requests to private IP addresses.
func WithAllowPrivateIPs() Option {
	return func(s *Server) {
		s.allowPrivateIPs = true
	}
}

// New creates a new Server with the given configuration and services.
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
	metrics.Init()

	mux := http.NewServeMux()

	s := &Server{
		mux:            mux,
		conf:           conf,
		authService:    authService,
		sessionService: sessionService,
		matcher:        matcher,
		rateLimiter:    rateLimiter,
		filterService:  filterService,
		orchestrator:   orchestrator,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.registerRoutes()

	maxBodyBytes := parseBytesSize(conf.MaxBodySize)
	handler := applyMiddlewares(mux,
		mw.Recover(),
		mw.RequestID(),
		mw.TracingMiddleware("straw"),
		mw.MetricsMiddleware(),
		mw.LoggerMiddleware(),
		mw.CORS(),
		mw.BodyLimit(maxBodyBytes),
	)

	s.server = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: headerTimeout,
	}

	return s
}

// Start begins serving HTTP requests.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.HTTPPort)

	s.server.Addr = addr
	if s.conf.Security.TLSCertFile != "" && s.conf.Security.TLSKeyFile != "" {
		return fmt.Errorf("listen and serve TLS: %w", s.server.ListenAndServeTLS(s.conf.Security.TLSCertFile, s.conf.Security.TLSKeyFile))
	}

	return fmt.Errorf("listen and serve: %w", s.server.ListenAndServe())
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	return fmt.Errorf("shutdown: %w", s.server.Shutdown(ctx))
}

// Address returns the address the server is configured to listen on.
func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.HTTPPort)
}

// GetMatcher returns the routing matcher.
func (s *Server) GetMatcher() *router.Matcher {
	return s.matcher
}

// GetMux returns the HTTP serve mux.
func (s *Server) GetMux() *http.ServeMux {
	return s.mux
}

// GetHandler returns the server's HTTP handler.
func (s *Server) GetHandler() http.Handler {
	return s.server.Handler
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", s.healthCheck)
	s.mux.HandleFunc("GET /readyz", s.readyCheck)

	authMiddleware := mw.AuthMiddleware(s.authService)

	concurrencyLimit := s.conf.MaxConcurrentRequests
	if concurrencyLimit <= 0 {
		concurrencyLimit = 50
	}

	concurrencyLimiter := mw.ConcurrencyLimiter(concurrencyLimit)
	sessionMiddleware := mw.SessionMiddleware(s.sessionService)

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

	v1Handler := applyMiddlewares(http.HandlerFunc(relayHandler.Handle),
		authMiddleware,
		sessionMiddleware,
		concurrencyLimiter,
	)
	s.mux.Handle("POST /v1/request", v1Handler)

	v2Handler := applyMiddlewares(http.HandlerFunc(relayHandler.Handle),
		authMiddleware,
		sessionMiddleware,
		concurrencyLimiter,
	)
	s.mux.Handle("POST /v2/request", v2Handler)
}

func (s *Server) healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) readyCheck(w http.ResponseWriter, _ *http.Request) {
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

const (
	kib = 1024
	mib = kib * kib
	gib = mib * kib

	headerTimeout = 30 * time.Second
)

func parseBytesSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0
	}

	var multiplier int64 = 1

	switch {
	case strings.HasSuffix(s, "G"):
		multiplier = gib
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "M"):
		multiplier = mib
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "K"):
		multiplier = kib
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "B"):
		s = s[:len(s)-1]
	}

	var val int64

	_, _ = fmt.Sscanf(s, "%d", &val)

	return val * multiplier
}

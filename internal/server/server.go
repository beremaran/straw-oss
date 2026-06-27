package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

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

type Server struct {
	mux            *http.ServeMux
	server         *http.Server
	conf           config.ServerConfig
	authService    *auth.Service
	sessionService *session.Service
	matcher        *router.Matcher
	rateLimiter    *ratelimit.RateLimiter
	filterService  *filter.Service
	orchestrator   *orchestrator.RetryExecutor

	allowPrivateIPs bool
}

type Option func(*Server)

func WithAllowPrivateIPs() Option {
	return func(s *Server) {
		s.allowPrivateIPs = true
	}
}

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
		Handler: handler,
	}

	return s
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

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) readyCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.conf.HTTPPort)
	s.server.Addr = addr
	if s.conf.Security.TLSCertFile != "" && s.conf.Security.TLSKeyFile != "" {
		return s.server.ListenAndServeTLS(s.conf.Security.TLSCertFile, s.conf.Security.TLSKeyFile)
	}
	return s.server.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.HTTPPort)
}

func (s *Server) GetMatcher() *router.Matcher {
	return s.matcher
}

func (s *Server) GetMux() *http.ServeMux {
	return s.mux
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

func parseBytesSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	var multiplier int64 = 1
	if strings.HasSuffix(s, "G") {
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "M") {
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "K") {
		multiplier = 1024
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "B") {
		s = s[:len(s)-1]
	}
	var val int64
	_, _ = fmt.Sscanf(s, "%d", &val)
	return val * multiplier
}

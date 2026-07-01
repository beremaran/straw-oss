// Package server provides the HTTP relay server.
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server/handlers"
	mw "github.com/beremaran/straw/internal/server/middleware"
)

// Server is the HTTP server for the Straw relay.
type Server struct {
	mux    *http.ServeMux
	server *http.Server
	conf   config.ServerConfig
	broker broker.MessageBroker
}

// New creates a relay server.
func New(conf config.ServerConfig, b broker.MessageBroker) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:    mux,
		conf:   conf,
		broker: b,
	}

	s.registerRoutes()

	handler := applyMiddlewares(mux,
		mw.Recover(),
		mw.RequestID(),
		mw.LoggerMiddleware(),
		mw.CORS(),
		mw.BodyLimit(parseBytesSize(conf.MaxBodySize)),
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

	var err error
	if s.conf.Security.TLSCertFile != "" && s.conf.Security.TLSKeyFile != "" {
		err = s.server.ListenAndServeTLS(s.conf.Security.TLSCertFile, s.conf.Security.TLSKeyFile)
	} else {
		err = s.server.ListenAndServe()
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	err := s.server.Shutdown(ctx)
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}

// Address returns the configured listen address.
func (s *Server) Address() string {
	return fmt.Sprintf(":%d", s.conf.HTTPPort)
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
	s.mux.HandleFunc("GET /healthz", healthCheck)
	s.mux.HandleFunc("GET /readyz", healthCheck)

	relayHandler := handlers.NewRelayHandler(
		s.broker,
		s.conf.EndpointID,
		[]byte(s.conf.Security.HMACSecret),
		s.conf.ResultTimeout,
		handlers.WithAllowPrivateIPs(s.conf.AllowPrivateIPs),
	)

	v1Handler := applyMiddlewares(
		http.HandlerFunc(relayHandler.Handle),
		mw.ConcurrencyLimiter(s.conf.MaxConcurrentRequests),
	)
	s.mux.Handle("POST /v1/request", v1Handler)
}

func healthCheck(w http.ResponseWriter, _ *http.Request) {
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

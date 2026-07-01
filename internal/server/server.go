// Package server provides the HTTP control server.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server/handlers"
	mw "github.com/beremaran/straw/internal/server/middleware"
)

type controlBroker interface {
	Publish(ctx context.Context, subject string, body []byte) error
	ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error)
}

// Server is the HTTP server for the Straw control.
type Server struct {
	mux    *http.ServeMux
	server *http.Server
	conf   config.ControlConfig
	broker controlBroker
}

// New creates a control server.
func New(conf config.ControlConfig, b controlBroker) *Server {
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
	s.server.Addr = fmt.Sprintf(":%d", s.conf.HTTPPort)
	if s.conf.Security.TLSCertFile != "" && s.conf.Security.TLSKeyFile != "" {
		return fmt.Errorf("listen TLS: %w", s.server.ListenAndServeTLS(s.conf.Security.TLSCertFile, s.conf.Security.TLSKeyFile))
	}

	return fmt.Errorf("listen: %w", s.server.ListenAndServe())
}

// Run loads configuration from the environment and starts the control server.
func Run(version string) error {
	cfg, err := config.LoadControlConfig()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	return RunWithConfig(cfg, version)
}

// RunWithConfig starts the control server with the provided configuration.
func RunWithConfig(cfg *config.ControlConfig, version string) error {
	slog.Info("starting straw control", "version", version, "egress_id", cfg.EgressID)

	natsBroker := broker.NewNatsBroker(cfg.NATS.URL, cfg.NATS.Token)

	err := natsBroker.Connect()
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}
	defer func() { _ = natsBroker.Close() }()

	err = natsBroker.DeclareStream(context.Background(), "tasks", "tasks.>")
	if err != nil {
		return fmt.Errorf("declare stream tasks: %w", err)
	}

	err = natsBroker.DeclareStream(context.Background(), "results", "results.>")
	if err != nil {
		return fmt.Errorf("declare stream results: %w", err)
	}

	s := New(*cfg, natsBroker)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	defer signal.Stop(c)

	go func() {
		err := s.Start()
		if err != nil {
			slog.Error("server stopped", "error", err)
		}
	}()

	fmt.Printf("Straw control %s started on %s, egress %s\n", version, s.Address(), cfg.EgressID)

	<-c

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	err = s.Stop(shutdownCtx)
	if err != nil {
		slog.Error("server forced to shutdown", "error", err)
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

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /healthz", healthCheck)
	s.mux.HandleFunc("GET /readyz", healthCheck)

	controlHandler := handlers.NewControlHandler(
		s.broker,
		s.conf.EgressID,
		s.conf.ResultTimeout,
		s.conf.AllowPrivateIPs,
	)

	v1Handler := applyMiddlewares(
		http.HandlerFunc(controlHandler.Handle),
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

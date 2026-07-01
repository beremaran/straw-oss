// Package server provides the HTTP control server.
package server

import (
	"context"
	"errors"
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
	CoreRequest(ctx context.Context, subject string, body []byte) ([]byte, error)
	CorePublish(ctx context.Context, subject string, body []byte) error
	CoreSubscribe(ctx context.Context, subject string, handler broker.Handler) (broker.Subscription, error)
}

// Server is the HTTP server for the Straw control.
type Server struct {
	mux     *http.ServeMux
	server  *http.Server
	proxy   *http.Server
	conf    config.ControlConfig
	broker  controlBroker
	initErr error
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

	if conf.ProxyPort != 0 {
		proxyHandler, err := handlers.NewProxyHandler(b, conf)
		if err != nil {
			s.initErr = err
		} else {
			s.proxy = &http.Server{
				Addr:              fmt.Sprintf(":%d", conf.ProxyPort),
				Handler:           proxyHandler,
				ReadHeaderTimeout: headerTimeout,
			}
		}
	}

	return s
}

// Start begins serving HTTP requests.
func (s *Server) Start() error {
	if s.initErr != nil {
		return s.initErr
	}

	if s.proxy != nil {
		go func() {
			err := s.proxy.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("proxy server stopped", "error", err)
			}
		}()
	}

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
	defer func() { _ = natsBroker.Close() }()

	err := natsBroker.Connect()
	if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}

	err = declareStreamWithTimeout(cfg, natsBroker, "tasks", "tasks.>")
	if err != nil {
		return fmt.Errorf("declare stream tasks: %w", err)
	}

	err = declareStreamWithTimeout(cfg, natsBroker, "results", "results.>")
	if err != nil {
		return fmt.Errorf("declare stream results: %w", err)
	}

	controlServer := New(*cfg, natsBroker)

	interruptSignalChannel := make(chan os.Signal, 1)

	signal.Notify(interruptSignalChannel, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interruptSignalChannel)

	go func() {
		err := controlServer.Start()
		if err != nil {
			slog.Error("server stopped", "error", err)
		}
	}()

	fmt.Printf("Straw control %s started on %s, egress %s\n", version, controlServer.Address(), cfg.EgressID)

	<-interruptSignalChannel

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	err = controlServer.Stop(shutdownCtx)
	if err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.proxy != nil {
		err := s.proxy.Shutdown(ctx)
		if err != nil {
			return fmt.Errorf("shutdown proxy: %w", err)
		}
	}

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
		s.conf.AuthToken,
		s.conf.Routes,
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
	headerTimeout = 30 * time.Second

	// Byte multipliers for size parsing.
	byteK = 1024
	byteM = 1024 * 1024
	byteG = 1024 * 1024 * 1024
)

func parseBytesSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return 0
	}

	var multiplier int64 = 1

	switch {
	case strings.HasSuffix(s, "G"):
		multiplier = byteG
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "M"):
		multiplier = byteM
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "K"):
		multiplier = byteK
		s = s[:len(s)-1]
	case strings.HasSuffix(s, "B"):
		s = s[:len(s)-1]
	}

	var val int64

	_, _ = fmt.Sscanf(s, "%d", &val)

	return val * multiplier
}

func declareStreamWithTimeout(cfg *config.ControlConfig, natsBroker *broker.NatsBroker, name string, subjects ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.StreamTimeout)
	defer cancel()

	err := natsBroker.DeclareStream(ctx, name, subjects...)
	if err != nil {
		return fmt.Errorf("declare stream tasks: %w", err)
	}

	return nil
}

// Package main runs the Straw control service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/beremaran/straw-oss/v2/internal/config"
	"github.com/beremaran/straw-oss/v2/internal/logging"
	"github.com/beremaran/straw-oss/v2/internal/natsx"
)

const (
	readHeaderTimeout       = 5 * time.Second
	shutdownTimeout         = 5 * time.Second
	healthcheckProbeTimeout = 2 * time.Second
	serverCount             = 2
)

var errHealthcheckNotReady = errors.New("healthcheck probe returned non-2xx status")

func main() {
	slog.SetDefault(logging.New("control"))

	err := run()
	if err != nil {
		slog.Error("control failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, healthcheck, err := loadControlConfig()
	if err != nil {
		return fmt.Errorf("load control config: %w", err)
	}

	if healthcheck {
		return runHealthcheck(cfg)
	}

	err = natsx.ValidateServers(cfg.NATS.Servers)
	if err != nil {
		return fmt.Errorf("validate nats servers: %w", err)
	}

	err = natsx.ValidateMaxPayload(cfg.NATS.MaxPayloadBytes, cfg.Transport.MaxFrameDataBytes, cfg.Request.MaxInlineRequestBodyBytes, cfg.Request.MaxInlineResponseBodyBytes)
	if err != nil {
		return fmt.Errorf("validate payload limits: %w", err)
	}

	natsConn, err := natsx.Connect(natsx.ConnectOptions{
		Servers:             cfg.NATS.Servers,
		UserCredentialsFile: cfg.NATS.UserCredentialsFile,
		Username:            os.Getenv(cfg.NATS.UsernameEnv),
		Password:            os.Getenv(cfg.NATS.PasswordEnv),
		ReconnectAttempts:   cfg.NATS.ReconnectAttempts,
		ReconnectWait:       time.Duration(cfg.NATS.ReconnectWaitMS) * time.Millisecond,
		PingInterval:        time.Duration(cfg.NATS.PingIntervalMS) * time.Millisecond,
		MaxPingFailures:     cfg.NATS.MaxPingFailures,
	})
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	defer func() { _ = natsConn.Drain() }()

	err = natsx.ValidateConnectedMaxPayload(natsConn, cfg.Transport.MaxFrameDataBytes, cfg.Request.MaxInlineRequestBodyBytes, cfg.Request.MaxInlineResponseBodyBytes)
	if err != nil {
		return fmt.Errorf("validate live nats payload limits: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return runDeploymentControl(ctx, cfg, natsConn)
}

func loadControlConfig() (config.ControlConfig, bool, error) {
	configPath := flag.String("config", "", "path to the control config file")
	healthcheck := flag.Bool("healthcheck", false, "probe the local readiness endpoint")

	flag.Parse()

	if *configPath == "" {
		return config.DefaultControl(), *healthcheck, nil
	}

	cfg, err := config.LoadControl(*configPath)
	if err != nil {
		return config.ControlConfig{}, false, fmt.Errorf("read %s: %w", *configPath, err)
	}

	return cfg, *healthcheck, nil
}

func runHealthcheck(cfg config.ControlConfig) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/readyz", cfg.Server.MetricsPort)

	ctx, cancel := context.WithTimeout(context.Background(), healthcheckProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build healthcheck request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck probe %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%w: %s status %d", errHealthcheckNotReady, url, resp.StatusCode)
	}

	return nil
}

func serveDeployment(ctx context.Context, cfg config.ControlConfig, api http.Handler, reg *prometheus.Registry) error {
	ready := &atomic.Bool{}
	ready.Store(true)

	apiServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.APIPort),
		Handler:           api,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	metricsServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.MetricsPort),
		Handler:           newMetricsMux(ready, reg),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errs := make(chan error, serverCount)
	go serveHTTP(apiServer, errs)
	go serveHTTP(metricsServer, errs)

	select {
	case <-ctx.Done():
		ready.Store(false)

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()

		err := apiServer.Shutdown(shutdownCtx)
		if err != nil {
			return fmt.Errorf("shutdown api server: %w", err)
		}

		err = metricsServer.Shutdown(shutdownCtx)
		if err != nil {
			return fmt.Errorf("shutdown metrics server: %w", err)
		}

		return nil
	case err := <-errs:
		return err
	}
}

func serveHTTP(server *http.Server, errs chan<- error) {
	slog.Info("listening", "addr", server.Addr)

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs <- fmt.Errorf("serve %s: %w", server.Addr, err)
	}
}

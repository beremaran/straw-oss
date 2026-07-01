package endpoint

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	endpointhttp "github.com/beremaran/straw/internal/endpoint/http"
	endpointtls "github.com/beremaran/straw/internal/endpoint/tls"
	endpointtransport "github.com/beremaran/straw/internal/endpoint/transport"
)

// Version is the build version of the endpoint worker.
var Version = "dev"

const defaultTimeout = 30 * time.Second

// Worker wraps the endpoint components.
type Worker struct {
	cfg      *config.EndpointConfig
	executor RequestExecutor
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// WithRequestExecutor configures the Worker to use a custom RequestExecutor.
func WithRequestExecutor(executor RequestExecutor) WorkerOption {
	return func(w *Worker) {
		w.executor = executor
	}
}

// NewWorker initializes a worker.
func NewWorker(cfg *config.EndpointConfig, opts ...WorkerOption) *Worker {
	w := &Worker{cfg: cfg}
	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Run loads configuration from the environment and starts a worker.
func Run() error {
	cfg, err := config.LoadEndpointConfig()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	return RunWithConfig(cfg)
}

// RunWithConfig starts the worker with the provided configuration.
func RunWithConfig(cfg *config.EndpointConfig) error {
	w := NewWorker(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return w.Start(ctx)
}

// Start consumes tasks until the context is canceled.
func (w *Worker) Start(ctx context.Context) error {
	slog.Info("starting endpoint worker",
		"endpoint_id", w.cfg.ID,
		"version", Version,
		"concurrency_limit", w.cfg.ConcurrencyLimit,
	)

	executor, cleanupExecutor := setupEndpointExecutor(w.cfg, w.executor)
	defer cleanupExecutor()

	mqBroker, err := connectWorkerBroker(w.cfg)
	if err != nil {
		return err
	}
	defer func() { _ = mqBroker.Close() }()

	resultPublisher := NewPublisher(mqBroker)
	taskConsumer := NewConsumer(
		mqBroker,
		executor,
		[]byte(w.cfg.Security.HMACSecret),
		w.cfg.ID,
		WithConcurrencyLimit(w.cfg.ConcurrencyLimit),
		WithResultHandler(resultPublisher.Handler()),
	)

	return taskConsumer.Start(ctx)
}

func connectWorkerBroker(cfg *config.EndpointConfig) (broker.MessageBroker, error) {
	mqBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)

	err := mqBroker.Connect()
	if err != nil {
		return nil, fmt.Errorf("connect to message broker: %w", err)
	}

	return mqBroker, nil
}

func setupEndpointExecutor(
	cfg *config.EndpointConfig,
	executor RequestExecutor,
) (RequestExecutor, func()) {
	if executor != nil {
		return executor, func() {}
	}

	registry := fingerprint.DefaultRegistry()

	poolConfig := endpointtransport.DefaultPoolConfig().
		WithMaxPoolHosts(cfg.MaxPoolHosts).
		WithIdleConnsPerHost(cfg.IdleConnsPerHost).
		WithIdleConnTimeout(cfg.IdleConnTimeout)

	pooledTransport := endpointtransport.NewPooledTransport(poolConfig, func(ctx context.Context, network, addr, fp string) (net.Conn, error) {
		return endpointtls.Dial(ctx, network, addr, fp)
	})
	httpClient := endpointhttp.NewClient(
		registry,
		pooledTransport,
		endpointhttp.WithEndpointID(cfg.ID),
		endpointhttp.WithDefaultTimeout(defaultTimeout),
	)

	return httpClient, func() {
		_ = httpClient.Close()
		_ = pooledTransport.Close()
	}
}

package egress

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
	"github.com/beremaran/straw/internal/egress/fingerprint"
	egresshttp "github.com/beremaran/straw/internal/egress/http"
	egresstls "github.com/beremaran/straw/internal/egress/tls"
	egresstransport "github.com/beremaran/straw/internal/egress/transport"
)

// Version is the build version of the egress worker.
var Version = "dev"

const defaultTimeout = 30 * time.Second

// Worker wraps the egress components.
type Worker struct {
	cfg      *config.EgressConfig
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
func NewWorker(cfg *config.EgressConfig, opts ...WorkerOption) *Worker {
	w := &Worker{cfg: cfg}
	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Run loads configuration from the environment and starts a worker.
func Run() error {
	cfg, err := config.LoadEgressConfig()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	return RunWithConfig(cfg)
}

// RunWithConfig starts the worker with the provided configuration.
func RunWithConfig(cfg *config.EgressConfig) error {
	w := NewWorker(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	return w.Start(ctx)
}

// Start consumes tasks until the context is canceled.
func (w *Worker) Start(ctx context.Context) error {
	slog.Info("starting egress worker",
		"egress_id", w.cfg.ID,
		"version", Version,
		"concurrency_limit", w.cfg.ConcurrencyLimit,
	)

	executor, cleanupExecutor := setupEgressExecutor(w.cfg, w.executor)
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

func connectWorkerBroker(cfg *config.EgressConfig) (broker.MessageBroker, error) {
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

func setupEgressExecutor(
	cfg *config.EgressConfig,
	executor RequestExecutor,
) (RequestExecutor, func()) {
	if executor != nil {
		return executor, func() {}
	}

	registry := fingerprint.DefaultRegistry()

	poolConfig := egresstransport.DefaultPoolConfig().
		WithMaxPoolHosts(cfg.MaxPoolHosts).
		WithIdleConnsPerHost(cfg.IdleConnsPerHost).
		WithIdleConnTimeout(cfg.IdleConnTimeout)

	pooledTransport := egresstransport.NewPooledTransport(poolConfig, func(ctx context.Context, network, addr, fp string) (net.Conn, error) {
		return egresstls.Dial(ctx, network, addr, fp)
	})
	httpClient := egresshttp.NewClient(
		registry,
		pooledTransport,
		egresshttp.WithEgressID(cfg.ID),
		egresshttp.WithDefaultTimeout(defaultTimeout),
	)

	return httpClient, func() {
		_ = httpClient.Close()
		_ = pooledTransport.Close()
	}
}

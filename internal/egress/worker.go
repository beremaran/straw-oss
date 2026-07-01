package egress

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	egresshttp "github.com/beremaran/straw/internal/egress/http"
)

// Version is the build version of the egress worker.
var Version = "dev"

const workerRoutines = 2

// Worker wraps the egress components.
type Worker struct {
	cfg      *config.EgressConfig
	executor RequestExecutor
}

// NewWorker initializes a worker.
func NewWorker(cfg *config.EgressConfig, executor RequestExecutor) *Worker {
	return &Worker{cfg: cfg, executor: executor}
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
	w := NewWorker(cfg, nil)

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
		w.cfg.ID,
		w.cfg.ConcurrencyLimit,
		resultPublisher.Publish,
	)
	tunnelConsumer := NewTunnelConsumer(mqBroker, w.cfg.ID, w.cfg.TunnelChunkSize)

	errCh := make(chan error, workerRoutines)
	go func() { errCh <- taskConsumer.Start(ctx) }()
	go func() { errCh <- tunnelConsumer.Start(ctx) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return nil
	}
}

func connectWorkerBroker(cfg *config.EgressConfig) (*broker.NatsBroker, error) {
	mqBroker := broker.NewNatsBroker(cfg.NATS.URL, cfg.NATS.Token)

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

	httpClient := egresshttp.NewClient(cfg.ID)

	return httpClient, func() {
		_ = httpClient.Close()
	}
}

package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	endpointhttp "github.com/beremaran/straw/internal/endpoint/http"
	endpointtls "github.com/beremaran/straw/internal/endpoint/tls"
	endpointtransport "github.com/beremaran/straw/internal/endpoint/transport"
	"github.com/beremaran/straw/internal/endpoint/update"
	"github.com/beremaran/straw/internal/observability/logging"
	obsmetrics "github.com/beremaran/straw/internal/observability/metrics"
	"github.com/beremaran/straw/internal/observability/tracing"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

var (
	// Version is the build version of the endpoint worker.
	Version = "dev"

	// ErrUnknownCommand is returned when an unrecognized control command is received.
	ErrUnknownCommand = errors.New("unknown command")
)

const (
	// heartbeatInterval is the frequency at which heartbeat messages are sent.
	heartbeatInterval = 10 * time.Second
	// shutdownTimeout is the maximum duration for health server shutdown.
	shutdownTimeout = 5 * time.Second
	// defaultTimeout is the default HTTP client request timeout.
	defaultTimeout = 30 * time.Second
	// updateTimeout is the maximum duration for an update installation.
	updateTimeout = 5 * time.Minute
	// shutdownGracePeriod is the maximum wait for in-flight tasks during shutdown.
	shutdownGracePeriod = 30 * time.Second
	// restartDelay is the pause before restarting the process.
	restartDelay = 500 * time.Millisecond
	// readHeaderTimeout is the maximum time for reading HTTP request headers.
	readHeaderTimeout = 10 * time.Second
	// tracerShutdownTimeout is the maximum duration for tracer provider shutdown.
	tracerShutdownTimeout = 5 * time.Second
)

// Worker wraps the endpoint components and manages their execution lifecycle.
type Worker struct {
	cfg        *config.EndpointConfig
	executor   RequestExecutor
	logHandler *ForwardingHandler
}

// WorkerOption defines a functional option for configuring a Worker.
type WorkerOption func(*Worker)

// WithRequestExecutor configures the Worker to use a custom RequestExecutor.
// If not provided, a default TLS-enabled HTTP client is used.
func WithRequestExecutor(executor RequestExecutor) WorkerOption {
	return func(w *Worker) {
		w.executor = executor
	}
}

// NewWorker initializes a new Worker with the given config and options.
func NewWorker(cfg *config.EndpointConfig, opts ...WorkerOption) *Worker {
	w := &Worker{
		cfg: cfg,
	}
	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Run loads the endpoint configuration from the environment and starts a default worker.
// It listens for system signals (SIGINT, SIGTERM) to initiate a graceful shutdown.
func Run() error {
	cfg, err := config.LoadEndpointConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	w := NewWorker(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	return w.Start(ctx)
}

// RunWithConfig starts the worker with the provided configuration.
// It listens for system signals (SIGINT, SIGTERM) to initiate a graceful shutdown.
func RunWithConfig(cfg *config.EndpointConfig) error {
	w := NewWorker(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	return w.Start(ctx)
}

// Start starts all background components of the worker and blocks until the context is canceled.
func (w *Worker) Start(ctx context.Context) error {
	cfg := w.cfg

	baseLogger := setupWorkerLogger(cfg)

	var brokerRef broker.MessageBroker

	w.logHandler = &ForwardingHandler{
		Handler:    baseLogger.Handler(),
		endpointID: cfg.ID,
		mu:         &sync.RWMutex{},
		brokerRef:  &brokerRef,
		enabled:    cfg.LogStreamEnabled,
	}
	logger := slog.New(w.logHandler)
	slog.SetDefault(logger)

	logWorkerStart(logger, cfg)

	defer setupEndpointTracer(ctx, logger)()

	executor, cleanupExecutor := setupEndpointExecutor(cfg, logger, w.executor)
	defer cleanupExecutor()

	mqBroker, cleanup := setupWorkerBroker(ctx, w, cfg, logger)
	defer cleanup()

	resultPublisher := NewPublisher(
		mqBroker,
		WithPublisherLogger(logger.WithGroup("publisher")),
	)
	hbSender := newWorkerHeartbeat(mqBroker, cfg, logger)
	taskConsumer := newWorkerConsumer(mqBroker, executor, cfg, logger, resultPublisher)
	updateChecker := newUpdateChecker(ctx, cfg, logger)

	var wg sync.WaitGroup

	obsmetrics.Init()

	healthServer := newHealthServer(cfg)

	startWorkerServices(ctx, &wg, logger, hbSender, updateChecker, taskConsumer, healthServer)

	w.handleControlCommands(ctx, mqBroker, taskConsumer, logger)

	<-ctx.Done()

	return shutdownWorker(ctx, &wg, logger, healthServer)
}

func setupWorkerLogger(cfg *config.EndpointConfig) *slog.Logger {
	return logging.SetupLogger(logging.Config{
		Level:   cfg.Observability.LogLevel,
		Format:  cfg.Observability.LogFormat,
		Service: "endpoint",
		Version: Version,
	})
}

func logWorkerStart(logger *slog.Logger, cfg *config.EndpointConfig) {
	logger.Info("starting endpoint worker",
		"endpoint_id", cfg.ID,
		"version", Version,
		"concurrency_limit", cfg.ConcurrencyLimit,
	)
}

func connectWorkerBroker(cfg *config.EndpointConfig) (broker.MessageBroker, error) {
	mqBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)

	err := mqBroker.Connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to message broker: %w", err)
	}

	return mqBroker, nil
}

func setupWorkerBroker(_ context.Context, w *Worker, cfg *config.EndpointConfig, logger *slog.Logger) (broker.MessageBroker, func()) {
	mqBroker, err := connectWorkerBroker(cfg)
	if err != nil {
		return nil, func() {}
	}

	cleanup := func() {
		w.logHandler.mu.Lock()
		*w.logHandler.brokerRef = nil
		w.logHandler.mu.Unlock()

		_ = mqBroker.Close()
	}

	w.logHandler.mu.Lock()
	*w.logHandler.brokerRef = mqBroker
	w.logHandler.mu.Unlock()

	logger.Info("connected to message broker")

	return mqBroker, cleanup
}

func newWorkerHeartbeat(b broker.MessageBroker, cfg *config.EndpointConfig, logger *slog.Logger) *HeartbeatSender {
	return NewHeartbeatSender(
		b,
		cfg.ID,
		WithHeartbeatVersion(Version),
		WithHeartbeatTags(cfg.Tags),
		WithHeartbeatInterval(heartbeatInterval),
		WithHeartbeatLogger(logger.WithGroup("heartbeat")),
	)
}

func newWorkerConsumer(
	b broker.MessageBroker,
	executor RequestExecutor,
	cfg *config.EndpointConfig,
	logger *slog.Logger,
	resultPublisher *Publisher,
) *Consumer {
	return NewConsumer(
		b,
		executor,
		[]byte(cfg.Security.HMACSecret),
		cfg.ID,
		WithConcurrencyLimit(cfg.ConcurrencyLimit),
		WithLogger(logger.WithGroup("consumer")),
		WithResultHandler(resultPublisher.Handler()),
	)
}

func newHealthServer(cfg *config.EndpointConfig) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Observability.MetricsPort),
		Handler:           setupHealthHandler(),
		ReadHeaderTimeout: readHeaderTimeout,
	}
}

func setupEndpointTracer(ctx context.Context, logger *slog.Logger) func() {
	shutdownTracer, err := tracing.InitTracerProvider(ctx, "straw-endpoint", Version)
	if err != nil {
		logger.Warn("failed to initialize tracer provider", "error", err)

		return func() {}
	}

	return func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, tracerShutdownTimeout)
		defer cancel()

		err := shutdownTracer(shutdownCtx)
		if err != nil {
			logger.Error("failed to shutdown tracer provider", "error", err)
		}
	}
}

func setupEndpointExecutor(
	cfg *config.EndpointConfig,
	logger *slog.Logger,
	executor RequestExecutor,
) (RequestExecutor, func()) {
	if executor != nil {
		return executor, func() {}
	}

	registry := fingerprint.DefaultRegistry()
	logger.Info("fingerprint registry initialized", "count", registry.Count())

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

func newUpdateChecker(ctx context.Context, cfg *config.EndpointConfig, logger *slog.Logger) *update.Checker {
	if !cfg.SelfUpdateEnabled || cfg.SelfUpdateURL == "" {
		return nil
	}

	installer := update.NewInstaller(
		update.WithInstallerLogger(logger.WithGroup("installer")),
	)

	return update.NewChecker(
		cfg.SelfUpdateURL,
		Version,
		update.WithCheckInterval(cfg.SelfUpdateInterval),
		update.WithCheckerLogger(logger.WithGroup("update")),
		update.WithUpdateCallback(updateCallback(ctx, logger, installer)),
	)
}

func updateCallback(ctx context.Context, logger *slog.Logger, installer *update.Installer) func(*update.Result) bool {
	return func(r *update.Result) bool {
		logger.Info("starting auto-update", "new_version", r.NewVersion)

		updateCtx, msgCancel := context.WithTimeout(ctx, updateTimeout)
		defer msgCancel()

		err := installer.Install(updateCtx, &update.VersionManifest{
			Version: r.NewVersion,
			URL:     r.DownloadURL,
			SHA256:  r.Checksum,
		})
		if err != nil {
			logger.Error("failed to install update", "error", err)

			return false
		}

		logger.Info("update installed, restarting...")

		err = installer.ReplaceAndRestart()
		if err != nil {
			logger.Error("failed to restart", "error", err)

			return false
		}

		return true
	}
}

func startWorkerServices(
	ctx context.Context,
	wg *sync.WaitGroup,
	logger *slog.Logger,
	hbSender *HeartbeatSender,
	updateChecker *update.Checker,
	taskConsumer *Consumer,
	healthServer *http.Server,
) {
	startWorkerService(wg, func() {
		hbSender.Start(ctx)
	})

	if updateChecker != nil {
		startWorkerService(wg, func() {
			updateChecker.Start(ctx)
		})
	}

	startWorkerService(wg, func() {
		err := taskConsumer.Start(ctx)
		if err != nil {
			logger.Error("consumer stopped with error", "error", err)
		}
	})
	startWorkerService(wg, func() {
		logger.Info("starting health/metrics server", "addr", healthServer.Addr)

		err := healthServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server failed", "error", err)
		}
	})
}

func startWorkerService(wg *sync.WaitGroup, run func()) {
	wg.Go(func() {
		run()
	})
}

func shutdownWorker(ctx context.Context, wg *sync.WaitGroup, logger *slog.Logger, healthServer *http.Server) error {
	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, shutdownTimeout)
	defer shutdownCancel()

	err := healthServer.Shutdown(shutdownCtx)
	if err != nil {
		logger.Warn("health server shutdown error", "error", err)
	}

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("shutdown complete")
	case <-time.After(shutdownGracePeriod):
		logger.Warn("shutdown timed out, forcing exit")
	}

	return nil
}

func setupHealthHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("/metrics", obsmetrics.Handler())
	obsmetrics.RegisterPprof(mux)

	return mux
}

func (w *Worker) handleControlCommands(ctx context.Context, mqBroker broker.MessageBroker, taskConsumer *Consumer, logger *slog.Logger) {
	subject := "endpoint.control." + w.cfg.ID
	logger.Info("subscribing to control commands", "subject", subject)

	err := mqBroker.Subscribe(ctx, subject, func(ctx context.Context, body []byte) error {
		var cmd protocol.ControlCommand

		err := json.Unmarshal(body, &cmd)
		if err != nil {
			logger.Error("failed to unmarshal control command", "error", err)

			return nil
		}

		logger.Info("received control command", "command_id", cmd.CommandID, "command", cmd.Command)

		w.publishCommandAck(ctx, mqBroker, cmd.CommandID, "acknowledged", "command received")
		w.publishCommandAck(ctx, mqBroker, cmd.CommandID, "running", "executing command")

		go executeControlCommand(ctx, mqBroker, cmd, taskConsumer, w, logger)

		return nil
	}, broker.WithTransient())
	if err != nil {
		logger.Error("failed to subscribe to control commands", "error", err)
	}
}

func executeControlCommand(ctx context.Context, mqBroker broker.MessageBroker, cmd protocol.ControlCommand, taskConsumer *Consumer, w *Worker, logger *slog.Logger) {
	var (
		err error
		msg string
	)

	switch cmd.Command {
	case "drain", "disable":
		taskConsumer.Drain()

		msg = cmd.Command + " complete"
	case "undrain", "enable":
		err = taskConsumer.Resume(ctx)
		msg = cmd.Command + " complete"
	case "restart":
		taskConsumer.Drain()
		w.publishCommandAck(ctx, mqBroker, cmd.CommandID, "succeeded", "restart initiated")
		time.Sleep(restartDelay)

		restartErr := update.NewInstaller(update.WithInstallerLogger(logger.WithGroup("restart"))).ReplaceAndRestart()
		if restartErr != nil {
			logger.Error("failed to restart", "error", restartErr)
		}

		os.Exit(0)

		return
	default:
		err = fmt.Errorf("%w: %s", ErrUnknownCommand, cmd.Command)
	}

	if err != nil {
		w.publishCommandAck(ctx, mqBroker, cmd.CommandID, "failed", err.Error())
	} else {
		w.publishCommandAck(ctx, mqBroker, cmd.CommandID, "succeeded", msg)
	}
}

func (w *Worker) publishCommandAck(ctx context.Context, mqBroker broker.MessageBroker, commandID string, status string, msg string) {
	ack := protocol.CommandAck{
		CommandID:  commandID,
		EndpointID: w.cfg.ID,
		Status:     status,
		Message:    msg,
		Timestamp:  time.Now().UTC(),
	}

	data, err := json.Marshal(ack)
	if err != nil {
		return
	}

	subject := "endpoint.control.ack." + commandID
	_ = mqBroker.Publish(ctx, subject, data)
}

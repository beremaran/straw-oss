package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/consumer"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/heartbeat"
	endpointhttp "github.com/kwilabs/straw-proxy-server/internal/endpoint/http"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/publisher"
	endpointtls "github.com/kwilabs/straw-proxy-server/internal/endpoint/tls"
	endpointtransport "github.com/kwilabs/straw-proxy-server/internal/endpoint/transport"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/update"
	"github.com/spf13/viper"
)

// Worker manages the background endpoint processes.
type Worker struct {
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	logger  *slog.Logger
	mu      sync.Mutex
	running bool
	stats   Stats
}

// Stats represents the worker statistics.
type Stats struct {
	TasksProcessed uint64
	TasksFailed    uint64
	BytesSent      uint64
	BytesReceived  uint64
	LatencySum     int64 // time.Duration in nanoseconds
	QueueDepth     int32
	Connected      int32 // 0 or 1
}

// NewWorker creates a new Worker instance.
func NewWorker(logger *slog.Logger) *Worker {
	return &Worker{
		logger: logger,
	}
}

// IsRunning returns true if the worker is currently running.
func (w *Worker) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

// GetStats returns the current worker statistics.
func (w *Worker) GetStats() Stats {
	// No need for a global lock if everything is atomic
	processed := atomic.LoadUint64(&w.stats.TasksProcessed)
	failed := atomic.LoadUint64(&w.stats.TasksFailed)
	sent := atomic.LoadUint64(&w.stats.BytesSent)
	received := atomic.LoadUint64(&w.stats.BytesReceived)
	latSum := atomic.LoadInt64(&w.stats.LatencySum)
	depth := atomic.LoadInt32(&w.stats.QueueDepth)
	conn := atomic.LoadInt32(&w.stats.Connected)

	return Stats{
		TasksProcessed: processed,
		TasksFailed:    failed,
		BytesSent:      sent,
		BytesReceived:  received,
		LatencySum:     latSum,
		QueueDepth:     depth,
		Connected:      conn,
	}
}

// Start starts the endpoint worker with the given configuration file path.
func (w *Worker) Start(configPath string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return errors.New("worker is already running")
	}

	// 1. Load Configuration
	// We need to support loading from a specific file or defaults if not provided (though GUI should ensure it exists or pass defaults)
	// For this implementation, we assume configPath points to a valid file.
	// If configPath is empty, it might try to load from default locations which we might want to avoid or control.
	// We'll reset Viper to ensure we don't have stale config.
	viper.Reset()

	cfg, err := config.LoadEndpointConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// 2. Setup Logger (we use the passed logger, but maybe we want to configure it based on config?
	// For GUI, we probably want to keep the GUI logger or redirect.
	// The CLI sets up a logger based on config. We'll stick with the GUI provided logger for now
	// or maybe update the log level if possible.
	// Let's just use the w.logger which ideally writes to the GUI console.

	w.logger.Info("starting endpoint worker",
		"endpoint_id", cfg.ID,
		"version", "dev",
		"concurrency_limit", cfg.ConcurrencyLimit,
	)

	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.running = true

	// 3. Initialize Components

	// fingerprint registry
	registry := fingerprint.DefaultRegistry()
	w.logger.Info("fingerprint registry initialized", "count", registry.Count())

	// Connection Pool
	poolConfig := endpointtransport.DefaultPoolConfig().
		WithMaxPoolHosts(cfg.MaxPoolHosts).
		WithIdleConnsPerHost(cfg.IdleConnsPerHost).
		WithIdleConnTimeout(cfg.IdleConnTimeout)

	pooledTransport := endpointtransport.NewPooledTransport(poolConfig, func(ctx context.Context, network, addr, fp string) (net.Conn, error) {
		return endpointtls.Dial(ctx, network, addr, fp)
	})
	// We can't defer Close here because it needs to stay open until Stop is called.
	// We'll handle closing in a cleanup goroutine or track it.
	// Ideally we could refactor struct to hold these, but for simplicity we'll let them live until context cancellation
	// or rely on GC/OS to clean up if we just crash, but for Start/Stop we need to clean up.
	// The transports and clients usually have Close() methods.
	// To keep it simple and effective, we'll use a go function that waits for ctx.Done to close them.

	// http client
	httpClient := endpointhttp.NewClient(
		registry,
		pooledTransport,
		endpointhttp.WithEndpointID(cfg.ID),
		endpointhttp.WithDefaultTimeout(30*time.Second),
	)

	// broker
	mqBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.Core.NatsURL),
		broker.Token(cfg.Core.NatsToken),
	)

	if err := mqBroker.Connect(); err != nil {
		cancel()
		w.running = false
		_ = httpClient.Close()
		_ = pooledTransport.Close()
		return fmt.Errorf("failed to connect to message broker: %w", err)
	}
	w.logger.Info("connected to message broker")

	// heartbeat sender
	hbSender := heartbeat.New(
		mqBroker,
		cfg.ID,
		heartbeat.WithVersion("dev"),
		heartbeat.WithTags(cfg.Tags),
		heartbeat.WithInterval(10*time.Second),
		heartbeat.WithLogger(w.logger.WithGroup("heartbeat")),
	)

	// Publisher
	resultPublisher := publisher.New(
		mqBroker,
		publisher.WithLogger(w.logger.WithGroup("publisher")),
	)

	// self-update checker
	var updateChecker *update.Checker
	if cfg.SelfUpdateEnabled && cfg.SelfUpdateURL != "" {
		installer := update.NewInstaller(
			update.WithInstallerLogger(w.logger.WithGroup("installer")),
		)

		updateChecker = update.NewChecker(
			cfg.SelfUpdateURL,
			"dev",
			update.WithCheckInterval(cfg.SelfUpdateInterval),
			update.WithCheckerLogger(w.logger.WithGroup("update")),
			update.WithUpdateCallback(func(r *update.Result) bool {
				w.logger.Info("starting auto-update", "new_version", r.NewVersion)
				// Simplified update logic for GUI - maybe just warn?
				// For now, keep it same as CLI but logging to GUI.
				updateCtx, msgCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer msgCancel()

				if err := installer.Install(updateCtx, &update.VersionManifest{
					Version: r.NewVersion,
					URL:     r.DownloadURL,
					SHA256:  r.Checksum,
				}); err != nil {
					w.logger.Error("failed to install update", "error", err)
					return false
				}

				w.logger.Info("update installed, restarting requires manual action in GUI for now...")
				// Note: Restarting a GUI app is trickier.
				// We might just let it update the binary and ask user to restart.
				return true
			}),
		)
	}

	// consumer
	taskConsumer := consumer.New(
		mqBroker,
		httpClient,
		[]byte(cfg.Security.HMACSecret),
		cfg.ID,
		consumer.WithConcurrencyLimit(cfg.ConcurrencyLimit),
		consumer.WithResultHandler(resultPublisher.Handler()),
		consumer.WithStatsCallback(func(res consumer.TaskResult) {
			atomic.AddUint64(&w.stats.TasksProcessed, 1)
			if res.HasError {
				atomic.AddUint64(&w.stats.TasksFailed, 1)
			}
			atomic.AddUint64(&w.stats.BytesSent, res.BytesSent)
			atomic.AddUint64(&w.stats.BytesReceived, res.BytesReceived)
			atomic.AddInt64(&w.stats.LatencySum, int64(res.Latency))
		}),
	)

	// 4. Start Background Services

	// Handle cleanup
	go func() {
		<-ctx.Done()
		w.logger.Info("context cancelled, cleaning up resources...")
		_ = mqBroker.Close()
		_ = httpClient.Close()
		_ = pooledTransport.Close()
		w.logger.Info("resources closed")
	}()

	// Start Heartbeat
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		hbSender.Start(ctx)
	}()

	// Start Update Checker
	if updateChecker != nil {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			updateChecker.Start(ctx)
		}()
	}

	// Start Consumer
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := taskConsumer.Start(ctx); err != nil {
			w.logger.Error("consumer stopped with error", "error", err)
			w.Stop() // Trigger full stop if consumer fails
		}
	}()

	// Start stats update loop
	atomic.StoreInt32(&w.stats.Connected, 1)
	w.wg.Add(1)
	go w.statsUpdateLoop(ctx, mqBroker, "endpoint."+cfg.ID+".tasks")

	return nil
}

func (w *Worker) statsUpdateLoop(ctx context.Context, b broker.MessageBroker, queueName string) {
	defer w.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			depth, err := b.QueueDepth(ctx, queueName)
			if err == nil {
				atomic.StoreInt32(&w.stats.QueueDepth, int32(depth))
			}
			if b.IsConnected() {
				atomic.StoreInt32(&w.stats.Connected, 1)
			} else {
				atomic.StoreInt32(&w.stats.Connected, 0)
			}
		}
	}
}

// Stop stops the worker.
func (w *Worker) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	w.logger.Info("stopping worker...")
	if w.cancel != nil {
		w.cancel()
	}

	// Wait for goroutines in background to avoid blocking UI?
	// We can spin up a goroutine to wait.
	go func() {
		w.wg.Wait()
		w.mu.Lock()
		w.running = false
		w.cancel = nil
		w.mu.Unlock()
		w.logger.Info("worker stopped")
	}()
}

// Package main implements the Straw relay entrypoint.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server"
	"github.com/beremaran/straw/pkg/broker"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func main() {
	ctx := context.Background()
	cfg := loadConfigOrDie()

	natsBroker := connectNATSOrDie(ctx, cfg)
	defer func() { _ = natsBroker.Close() }()

	relayServer := server.New(*cfg, natsBroker)
	go func() {
		err := relayServer.Start()
		if err != nil {
			slog.Error("server stopped", "error", err)
		}
	}()

	fmt.Printf("Straw relay %s started on %s, endpoint %s\n", Version, relayServer.Address(), cfg.EndpointID)

	listenInterrupts()
	shutdown(ctx, cfg, relayServer)
}

func loadConfigOrDie() *config.ServerConfig {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	return cfg
}

func connectNATSOrDie(ctx context.Context, cfg *config.ServerConfig) *broker.NatsBroker {
	natsBroker := broker.NewNatsBroker(
		broker.Addrs(cfg.NATS.URL),
		broker.Token(cfg.NATS.Token),
	)

	err := natsBroker.Connect()
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}

	declareStreamOrDie(ctx, natsBroker, "tasks", "tasks.>")
	declareStreamOrDie(ctx, natsBroker, "results", "results.>")

	return natsBroker
}

func declareStreamOrDie(ctx context.Context, natsBroker *broker.NatsBroker, name string, subjects ...string) {
	err := natsBroker.DeclareStream(ctx, name, subjects...)
	if err != nil {
		slog.Error("failed to declare NATS stream", "stream", name, "error", err)
		os.Exit(1)
	}
}

func listenInterrupts() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

func shutdown(ctx context.Context, cfg *config.ServerConfig, srv *server.Server) {
	ctx, cancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancel()

	err := srv.Stop(ctx)
	if err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}
}

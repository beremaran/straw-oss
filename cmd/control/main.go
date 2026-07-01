// Package main implements the Straw control entrypoint.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/server"
)

var Version = "dev"

func main() {
	ctx := context.Background()
	cfg := loadConfigOrDie()

	natsBroker := connectNATSOrDie(ctx, cfg)
	defer func() { _ = natsBroker.Close() }()

	controlServer := server.New(*cfg, natsBroker)
	go func() {
		err := controlServer.Start()
		if err != nil {
			slog.Error("server stopped", "error", err)
		}
	}()

	fmt.Printf("Straw control %s started on %s, egress %s\n", Version, controlServer.Address(), cfg.EgressID)

	listenInterrupts()
	shutdown(ctx, cfg, controlServer)
}

func loadConfigOrDie() *config.ControlConfig {
	cfg, err := config.LoadControlConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	return cfg
}

func connectNATSOrDie(ctx context.Context, cfg *config.ControlConfig) *broker.NatsBroker {
	natsBroker := broker.NewNatsBroker(cfg.NATS.URL, cfg.NATS.Token)

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

func shutdown(ctx context.Context, cfg *config.ControlConfig, srv *server.Server) {
	ctx, cancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer cancel()

	err := srv.Stop(ctx)
	if err != nil {
		slog.Error("server forced to shutdown", "error", err)
	}
}

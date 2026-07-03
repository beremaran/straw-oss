// Package main runs the Straw egress worker.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/beremaran/straw/v2/internal/config"
	"github.com/beremaran/straw/v2/internal/egress"
	"github.com/beremaran/straw/v2/internal/natsx"
)

const (
	exitUsage          = 2
	defaultConcurrency = 4
)

func main() {
	err := run()
	if err != nil {
		log.Fatalf("egress: %v", err)
	}
}

func run() error {
	configPath := flag.String("config", "", "path to the egress config file")

	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing required -config flag")
		os.Exit(exitUsage)
	}

	egressConfig, err := config.LoadEgress(*configPath)
	if err != nil {
		return fmt.Errorf("load egress config: %w", err)
	}

	err = natsx.ValidateServers(egressConfig.NATS.Servers)
	if err != nil {
		return fmt.Errorf("validate nats servers: %w", err)
	}

	natsConn, err := natsx.Connect(natsx.ConnectOptions{
		Servers:             egressConfig.NATS.Servers,
		UserCredentialsFile: egressConfig.NATS.UserCredentialsFile,
		ReconnectAttempts:   egressConfig.NATS.ReconnectAttempts,
		ReconnectWait:       time.Duration(egressConfig.NATS.ReconnectWaitMS) * time.Millisecond,
		PingInterval:        time.Duration(egressConfig.NATS.PingIntervalMS) * time.Millisecond,
		MaxPingFailures:     egressConfig.NATS.MaxPingFailures,
	})
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		if natsConn != nil {
			drainErr := natsConn.Drain()
			if drainErr != nil {
				log.Printf("egress: drain nats connection: %v", drainErr)
			}
		}
	}()

	log.Printf("egress: connected to %s", natsConn.ConnectedUrlRedacted())

	return runWorker(ctx, natsConn, egressConfig)
}

func runWorker(ctx context.Context, natsConn *natsx.Connection, cfg config.EgressConfig) error {
	// Generate the worker's ed25519 identity keypair.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate worker key: %w", err)
	}

	id := egress.Identity{
		WorkerID:     cfg.WorkerID,
		CredentialID: cfg.CredentialID,
		ExecutorType: "egress",
		PrivateKey:   priv,
	}

	caps := egress.Capabilities{
		SoftwareVersion: "dev",
		MaxConcurrency:  defaultConcurrency,
	}

	heartbeatInterval := time.Duration(cfg.HeartbeatIntervalMs) * time.Millisecond

	log.Printf("egress: starting run loop (worker=%s, heartbeat=%v)", cfg.WorkerID, heartbeatInterval)

	err = egress.Run(ctx, natsConn, id, caps, heartbeatInterval)
	if err != nil {
		return fmt.Errorf("egress run loop: %w", err)
	}

	return nil
}

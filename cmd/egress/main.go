// Package main runs the Straw egress worker.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
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

var (
	errPrivateKeyEnvUnset   = errors.New("configured private key environment variable is unset or empty")
	errPrivateKeyInvalidLen = errors.New("invalid ed25519 key length")
)

// loadWorkerPrivateKey reads and decodes the worker's persistent ed25519
// identity key from the environment variable named by cfg.PrivateKeyEd25519Env
// (base64-standard-encoded 32-byte seed or 64-byte full private key). A
// persistent, configured key is required so a live worker's signature can
// match a pre-seeded credential's public key (docs/planning/27); Control
// verifies every registration against the credential's stored public key.
func loadWorkerPrivateKey(cfg config.EgressConfig) (ed25519.PrivateKey, error) {
	encoded := os.Getenv(cfg.PrivateKeyEd25519Env)
	if encoded == "" {
		return nil, fmt.Errorf("%w: %s", errPrivateKeyEnvUnset, cfg.PrivateKeyEd25519Env)
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", cfg.PrivateKeyEd25519Env, err)
	}

	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	default:
		return nil, fmt.Errorf("%s: %w: %d", cfg.PrivateKeyEd25519Env, errPrivateKeyInvalidLen, len(raw))
	}
}

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
	priv, err := loadWorkerPrivateKey(cfg)
	if err != nil {
		return fmt.Errorf("load worker private key: %w", err)
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

	executor := egress.NewExecutor(egress.ExecutorOptions{})

	log.Printf("egress: starting run loop (worker=%s, heartbeat=%v)", cfg.WorkerID, heartbeatInterval)

	err = egress.Run(ctx, natsConn, id, caps, executor, heartbeatInterval)
	if err != nil {
		return fmt.Errorf("egress run loop: %w", err)
	}

	return nil
}

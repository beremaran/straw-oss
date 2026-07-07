// Package main implements a static-response example Egress worker built
// purely on sdk/egress and the standard library. It proves that a third
// party can implement a custom Egress node with only the public SDK
// (docs/tasks/p2/13-example-custom-egress.md); see README.md for the
// operator obligations a custom implementation must uphold.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"

	sdkegress "github.com/beremaran/straw/v2/sdk/egress"
)

const (
	defaultHeartbeatInterval = 5 * time.Second
	defaultMaxConcurrency    = 4
	defaultStatus            = 200
	defaultBody              = "static-response\n"
	defaultWorkerID          = "wrk_egress_static_example"
	defaultWorkerRefValue    = "wrk-static-example-ref"
	defaultNATSServers       = "nats://127.0.0.1:4222"

	// executorType is the ExecutorType claim this example advertises at
	// registration; it identifies the implementation, not a vendor.
	executorType = "egress-static-example"

	// privateKeyEnv names the environment variable holding the worker's
	// persistent ed25519 identity key (base64-standard, 32-byte seed or
	// 64-byte full key), matching cmd/egress's convention so the registered
	// public key can be pre-seeded against a Control credential.
	privateKeyEnv = "STRAW_EGRESS_STATIC_PRIVATE_KEY_B64"
)

var errPrivateKeyInvalidLength = errors.New("invalid ed25519 private key length")

type workerConfig struct {
	natsServers    string
	workerID       string
	credentialID   string
	maxConcurrency uint32
	status         uint32
	body           string
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	err := run()
	if err != nil {
		slog.Error("egress-static failed", "error", err)
		os.Exit(1)
	}
}

// parseFlags reads the worker's flags. max-concurrency and status are parsed
// through strconv.ParseUint with an explicit 32-bit width so the uint32
// fields they populate can never silently overflow.
func parseFlags() (workerConfig, error) {
	cfg := workerConfig{maxConcurrency: defaultMaxConcurrency, status: defaultStatus}

	flag.StringVar(&cfg.natsServers, "nats-servers", defaultNATSServers, "comma-separated NATS server URLs")
	flag.StringVar(&cfg.workerID, "worker-id", defaultWorkerID, "worker id to register as")
	flag.StringVar(&cfg.credentialID, "credential-id", defaultWorkerRefValue, "credential id to register with")
	flag.StringVar(&cfg.body, "body", defaultBody, "response body returned for every assignment")

	var flagErr error

	flag.Func("max-concurrency", "max concurrent assignments to advertise", func(s string) error {
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			flagErr = fmt.Errorf("max-concurrency: %w", err)

			return flagErr
		}

		cfg.maxConcurrency = uint32(v)

		return nil
	})
	flag.Func("status", "HTTP status code returned for every assignment", func(s string) error {
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			flagErr = fmt.Errorf("status: %w", err)

			return flagErr
		}

		cfg.status = uint32(v)

		return nil
	})

	flag.Parse()

	return cfg, flagErr
}

func run() error {
	cfg, err := parseFlags()
	if err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	priv, pub, err := loadOrGeneratePrivateKey()
	if err != nil {
		return fmt.Errorf("load private key: %w", err)
	}

	slog.Info("worker identity", "worker_id", cfg.workerID, "public_key_b64", base64.StdEncoding.EncodeToString(pub))

	conn, err := nats.Connect(cfg.natsServers, nats.MaxReconnects(-1))
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}

	defer drainConn(conn)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	id := sdkegress.Identity{
		WorkerID:     cfg.workerID,
		CredentialID: cfg.credentialID,
		ExecutorType: executorType,
		PrivateKey:   priv,
	}
	caps := sdkegress.Capabilities{
		MaxConcurrency:  cfg.maxConcurrency,
		SoftwareVersion: executorType,
	}
	executor := &staticExecutor{status: cfg.status, body: []byte(cfg.body)}
	ready := &atomic.Bool{}

	slog.Info("starting egress-static worker", "nats_servers", cfg.natsServers, "worker_id", cfg.workerID)

	err = sdkegress.Run(ctx, conn, id, caps, defaultHeartbeatInterval, ready, newAssignmentFactory(conn, id, executor))
	if err != nil {
		return fmt.Errorf("run worker: %w", err)
	}

	return nil
}

// newAssignmentFactory builds the sdkegress.AssignmentFactory that binds a
// registered session to the static executor.
func newAssignmentFactory(conn *nats.Conn, id sdkegress.Identity, executor sdkegress.Executor) sdkegress.AssignmentFactory {
	return func(sessionID string, maxConcurrency uint32) (sdkegress.AssignmentServer, error) {
		worker, err := sdkegress.NewWorker(sdkegress.WorkerOptions{
			Conn:           conn,
			Identity:       id,
			Executor:       executor,
			SessionID:      sessionID,
			MaxConcurrency: maxConcurrency,
		})
		if err != nil {
			return nil, fmt.Errorf("new worker: %w", err)
		}

		return worker, nil
	}
}

func drainConn(conn *nats.Conn) {
	err := conn.Drain()
	if err != nil {
		slog.Warn("drain nats connection failed", "error", err)
	}
}

// loadOrGeneratePrivateKey reads the worker's persistent ed25519 identity key
// from privateKeyEnv, or generates an ephemeral one for local experimentation
// when unset. A real deployment must configure a persistent key so the
// registered public key matches a pre-seeded Control credential.
func loadOrGeneratePrivateKey() (ed25519.PrivateKey, ed25519.PublicKey, error) {
	encoded := os.Getenv(privateKeyEnv)
	if encoded == "" {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
		}

		slog.Warn("no persistent private key configured, generated an ephemeral one", "env", privateKeyEnv)

		return priv, pub, nil
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, fmt.Errorf("decode %s: %w", privateKeyEnv, err)
	}

	switch len(raw) {
	case ed25519.SeedSize:
		priv := ed25519.NewKeyFromSeed(raw)

		pub, ok := priv.Public().(ed25519.PublicKey)
		if !ok {
			return nil, nil, fmt.Errorf("%s: %w", privateKeyEnv, errPrivateKeyInvalidLength)
		}

		return priv, pub, nil
	case ed25519.PrivateKeySize:
		priv := ed25519.PrivateKey(raw)

		pub, ok := priv.Public().(ed25519.PublicKey)
		if !ok {
			return nil, nil, fmt.Errorf("%s: %w", privateKeyEnv, errPrivateKeyInvalidLength)
		}

		return priv, pub, nil
	default:
		return nil, nil, fmt.Errorf("%s: %w: %d", privateKeyEnv, errPrivateKeyInvalidLength, len(raw))
	}
}

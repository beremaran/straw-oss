package egress

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/protocol"
	"github.com/beremaran/straw/internal/protocol/wirepb"
)

const (
	defaultTunnelChunkSize = 16 * 1024
	defaultDialTimeout     = 10 * time.Second
)

type tunnelBroker interface {
	CorePublish(ctx context.Context, subject string, body []byte) error
	CoreSubscribe(ctx context.Context, subject string, handler broker.Handler) (broker.Subscription, error)
	CoreSubscribeRequest(ctx context.Context, subject string, handler broker.RequestHandler) (broker.Subscription, error)
}

// TunnelConsumer consumes tunnel open requests and forwards raw TCP bytes.
type TunnelConsumer struct {
	broker      tunnelBroker
	egressID    string
	chunkSize   int
	dialTimeout time.Duration
	logger      *slog.Logger
}

// NewTunnelConsumer creates a raw TCP tunnel consumer.
func NewTunnelConsumer(b tunnelBroker, egressID string, chunkSize int) *TunnelConsumer {
	if chunkSize <= 0 {
		chunkSize = defaultTunnelChunkSize
	}

	return &TunnelConsumer{
		broker:      b,
		egressID:    egressID,
		chunkSize:   chunkSize,
		dialTimeout: defaultDialTimeout,
		logger:      slog.Default(),
	}
}

// Start subscribes to tunnel open requests until ctx is canceled.
func (c *TunnelConsumer) Start(ctx context.Context) error {
	subject := "tunnels." + c.egressID + ".open"

	sub, err := c.broker.CoreSubscribeRequest(ctx, subject, c.handleOpen)
	if err != nil {
		return fmt.Errorf("subscribe tunnel open: %w", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	<-ctx.Done()

	return nil
}

func (c *TunnelConsumer) handleOpen(ctx context.Context, body []byte) ([]byte, error) {
	open, err := protocol.UnmarshalTunnelOpen(body)
	if err != nil {
		return nil, fmt.Errorf("unmarshal tunnel open: %w", err)
	}

	result := &wirepb.TunnelOpenResult{TunnelId: open.GetTunnelId()}
	dialer := net.Dialer{Timeout: c.dialTimeout}

	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(open.GetHost(), strconv.Itoa(int(open.GetPort()))))
	if err != nil {
		result.Error = &wirepb.ErrorInfo{Code: protocol.ErrCodeUpstreamError, Message: err.Error(), Retryable: true}

		reply, marshalErr := protocol.MarshalTunnelOpenResult(result)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshal tunnel open error result: %w", marshalErr)
		}

		return reply, nil
	}

	go c.runTunnel(ctx, open.GetTunnelId(), conn)

	reply, err := protocol.MarshalTunnelOpenResult(result)
	if err != nil {
		return nil, fmt.Errorf("marshal tunnel open result: %w", err)
	}

	return reply, nil
}

func (c *TunnelConsumer) runTunnel(ctx context.Context, tunnelID string, conn net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = conn.Close() }()

	c2e := "tunnels." + tunnelID + ".c2e"
	e2c := "tunnels." + tunnelID + ".e2c"
	closeSubject := "tunnels." + tunnelID + ".close"

	sub, err := c.broker.CoreSubscribe(ctx, c2e, func(_ context.Context, body []byte) error {
		chunk, err := protocol.UnmarshalTunnelChunk(body)
		if err != nil {
			return fmt.Errorf("unmarshal tunnel chunk: %w", err)
		}

		_, err = conn.Write(chunk.GetData())
		if err != nil {
			return fmt.Errorf("write tunnel tcp: %w", err)
		}

		return nil
	})
	if err != nil {
		c.logger.Error("failed to subscribe tunnel c2e", "tunnel_id", tunnelID, "error", err)

		return
	}
	defer func() { _ = sub.Unsubscribe() }()

	closeSub, err := c.broker.CoreSubscribe(ctx, closeSubject, func(_ context.Context, _ []byte) error {
		cancel()

		return nil
	})
	if err == nil {
		defer func() { _ = closeSub.Unsubscribe() }()
	}

	var seq atomic.Uint64

	var wg sync.WaitGroup
	wg.Go(func() {
		c.copyTCPToNATS(ctx, cancel, conn, tunnelID, e2c, &seq)
	})

	<-ctx.Done()

	_ = conn.Close()

	wg.Wait()
}

func (c *TunnelConsumer) copyTCPToNATS(
	ctx context.Context,
	cancel context.CancelFunc,
	conn net.Conn,
	tunnelID string,
	subject string,
	seq *atomic.Uint64,
) {
	buf := make([]byte, c.chunkSize)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			c.publishTunnelChunk(ctx, subject, tunnelID, seq.Add(1), buf[:n])
		}

		if err != nil {
			if !errors.Is(err, io.EOF) {
				c.logger.Debug("tunnel read stopped", "tunnel_id", tunnelID, "error", err)
			}

			cancel()

			return
		}
	}
}

func (c *TunnelConsumer) publishTunnelChunk(ctx context.Context, subject string, tunnelID string, seq uint64, data []byte) {
	msg, err := protocol.MarshalTunnelChunk(&wirepb.TunnelChunk{
		TunnelId: tunnelID,
		Seq:      seq,
		Data:     append([]byte(nil), data...),
	})
	if err != nil {
		return
	}

	_ = c.broker.CorePublish(ctx, subject, msg)
}

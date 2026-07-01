// Package egress provides egress task consumption and result publishing.
package egress

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/protocol"
	"github.com/beremaran/straw/internal/protocol/wirepb"
)

const (
	// DefaultConcurrencyLimit is the default maximum number of concurrent tasks.
	DefaultConcurrencyLimit = 25
)

type subscriber interface {
	Subscribe(ctx context.Context, subject string, handler broker.Handler, maxAckPending int) error
}

// RequestExecutor executes a protobuf proxy request.
type RequestExecutor interface {
	Do(ctx context.Context, req *wirepb.Request) (*wirepb.Response, error)
}

// Consumer manages task consumption from a message broker and execution of proxy requests.
type Consumer struct {
	broker           subscriber
	httpClient       RequestExecutor
	egressID         string
	taskSubject      string
	concurrencyLimit int
	semaphore        chan struct{}
	wg               sync.WaitGroup
	resultHandler    func(ctx context.Context, resp *wirepb.Response, replyTo string) error
	logger           *slog.Logger
}

// NewConsumer creates a new Consumer with the given broker and executor.
func NewConsumer(
	b subscriber,
	httpClient RequestExecutor,
	egressID string,
	concurrencyLimit int,
	resultHandler func(ctx context.Context, resp *wirepb.Response, replyTo string) error,
) *Consumer {
	if concurrencyLimit <= 0 {
		concurrencyLimit = DefaultConcurrencyLimit
	}

	c := &Consumer{
		broker:           b,
		httpClient:       httpClient,
		egressID:         egressID,
		taskSubject:      "tasks." + egressID + ".tasks",
		concurrencyLimit: concurrencyLimit,
		resultHandler:    resultHandler,
		logger:           slog.Default(),
	}

	c.semaphore = make(chan struct{}, c.concurrencyLimit)

	return c
}

// Start begins consuming tasks and blocks until the context is canceled.
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("starting consumer",
		"egress_id", c.egressID,
		"subject", c.taskSubject,
		"concurrency_limit", c.concurrencyLimit,
	)

	err := c.broker.Subscribe(ctx, c.taskSubject, c.handleMessage, c.concurrencyLimit)
	if err != nil {
		return fmt.Errorf("subscribe to %s: %w", c.taskSubject, err)
	}

	<-ctx.Done()

	c.wg.Wait()

	c.logger.Info("consumer stopped", "egress_id", c.egressID)

	return nil
}

// TaskError represents a recoverable task processing error.
type TaskError struct {
	Code    string
	Message string
}

func (e *TaskError) Error() string {
	return e.Code + ": " + e.Message
}

func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
	select {
	case c.semaphore <- struct{}{}:
	case <-ctx.Done():
		return fmt.Errorf("context done: %w", ctx.Err())
	}

	c.wg.Go(func() {
		defer func() { <-c.semaphore }()

		err := c.processTask(ctx, body)
		if err != nil {
			c.logger.Error("failed to process task",
				"error", err,
				"egress_id", c.egressID,
			)
		}
	})

	return nil
}

func (c *Consumer) processTask(ctx context.Context, body []byte) error {
	req, err := c.parseTask(body)
	if err != nil {
		return err
	}

	c.logger.Info("processing task",
		"request_id", req.GetId(),
		"method", req.GetMethod(),
		"url", req.GetUrl(),
	)

	resp := c.executeRequest(ctx, req)
	c.recordTaskCompletion(req, resp)

	return c.publishResult(ctx, req, resp)
}

func (c *Consumer) parseTask(body []byte) (*wirepb.Request, error) {
	req, err := protocol.UnmarshalRequest(body)
	if err != nil {
		c.logger.Warn("failed to unmarshal task",
			"error", err,
			"body_len", len(body),
		)

		return nil, &TaskError{
			Code:    protocol.ErrCodeInternalError,
			Message: "failed to unmarshal task: " + err.Error(),
		}
	}

	return req, nil
}

func (c *Consumer) executeRequest(ctx context.Context, req *wirepb.Request) *wirepb.Response {
	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		c.logger.Error("HTTP request failed",
			"request_id", req.GetId(),
			"error", err,
		)

		resp = &wirepb.Response{
			RequestId: req.GetId(),
			EgressId:  c.egressID,
			Error: &wirepb.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   err.Error(),
				Retryable: true,
			},
		}
	}

	resp.EgressId = c.egressID

	return resp
}

func (c *Consumer) recordTaskCompletion(req *wirepb.Request, resp *wirepb.Response) {
	c.logger.Info("task completed",
		"request_id", req.GetId(),
		"status_code", resp.GetStatusCode(),
		"has_error", resp.GetError() != nil,
	)
}

func (c *Consumer) publishResult(ctx context.Context, req *wirepb.Request, resp *wirepb.Response) error {
	if c.resultHandler != nil {
		err := c.resultHandler(ctx, resp, req.GetReplyTo())
		if err != nil {
			c.logger.Error("failed to publish result",
				"request_id", req.GetId(),
				"error", err,
			)

			return err
		}
	}

	return nil
}

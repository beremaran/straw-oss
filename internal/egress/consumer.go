// Package egress provides egress task consumption and result publishing.
package egress

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/beremaran/straw/internal/broker"
	"github.com/beremaran/straw/internal/protocol"
)

const (
	// DefaultConcurrencyLimit is the default maximum number of concurrent tasks.
	DefaultConcurrencyLimit = 25
)

// RequestExecutor defines the interface for executing a proxy request.
// This allows users to supply their own request executors or HTTP clients.
type RequestExecutor interface {
	Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
}

// Consumer manages task consumption from a message broker and execution of proxy requests.
type Consumer struct {
	broker           broker.MessageBroker
	httpClient       RequestExecutor
	egressID         string
	taskSubject      string
	concurrencyLimit int
	semaphore        chan struct{}
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	resultHandler    func(ctx context.Context, resp *protocol.Response, replyTo string) error
	logger           *slog.Logger
	mu               sync.Mutex
	subCtx           context.Context
	subCancel        context.CancelFunc
}

// Option configures a Consumer.
type Option func(*Consumer)

// WithConcurrencyLimit sets the maximum number of concurrent tasks the Consumer will process.
func WithConcurrencyLimit(n int) Option {
	return func(c *Consumer) {
		if n > 0 {
			c.concurrencyLimit = n
		}
	}
}

// WithResultHandler sets a callback invoked when a task result is ready to be published.
func WithResultHandler(h func(ctx context.Context, resp *protocol.Response, replyTo string) error) Option {
	return func(c *Consumer) {
		c.resultHandler = h
	}
}

// WithLogger sets the logger used by the Consumer.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Consumer) {
		c.logger = logger
	}
}

// NewConsumer creates a new Consumer with the given broker, executor, and options.
func NewConsumer(
	b broker.MessageBroker,
	httpClient RequestExecutor,
	egressID string,
	opts ...Option,
) *Consumer {
	c := &Consumer{
		broker:           b,
		httpClient:       httpClient,
		egressID:         egressID,
		taskSubject:      "tasks." + egressID + ".tasks",
		concurrencyLimit: DefaultConcurrencyLimit,
		logger:           slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	c.semaphore = make(chan struct{}, c.concurrencyLimit)

	return c
}

// Start begins consuming tasks and blocks until the context is canceled.
func (c *Consumer) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.logger.Info("starting consumer",
		"egress_id", c.egressID,
		"subject", c.taskSubject,
		"concurrency_limit", c.concurrencyLimit,
	)

	err := c.Resume(ctx)
	if err != nil {
		return err
	}

	<-c.ctx.Done()

	c.Drain()

	c.logger.Info("consumer stopped", "egress_id", c.egressID)

	return nil
}

// Stop signals the Consumer to stop consuming tasks.
func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// Resume subscribes the Consumer to its task subject if not already subscribed.
func (c *Consumer) Resume(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.subCancel != nil {
		return nil
	}

	c.logger.Info("subscribing task consumer", "subject", c.taskSubject)

	subCtx, subCancel := context.WithCancel(ctx)
	c.subCtx = subCtx
	c.subCancel = subCancel

	err := c.broker.Subscribe(subCtx, c.taskSubject, c.handleMessage, broker.WithMaxAckPending(c.concurrencyLimit))
	if err != nil {
		subCancel()
		c.subCancel = nil

		return fmt.Errorf("subscribe to %s: %w", c.taskSubject, err)
	}

	return nil
}

// Drain unsubscribes the Consumer and waits for in-flight tasks to complete.
func (c *Consumer) Drain() {
	c.mu.Lock()
	if c.subCancel != nil {
		c.logger.Info("draining task consumer (unsubscribing)", "subject", c.taskSubject)
		c.subCancel()
		c.subCancel = nil
	}
	c.mu.Unlock()

	c.wg.Wait()
}

// TaskError represents a recoverable task processing error.
type TaskError struct {
	Code    string
	Message string
}

func (e *TaskError) Error() string {
	return e.Code + ": " + e.Message
}

// TaskSubject returns the NATS subject on which tasks are consumed.
func (c *Consumer) TaskSubject() string {
	return c.taskSubject
}

// ConcurrencyLimit returns the maximum number of concurrent tasks.
func (c *Consumer) ConcurrencyLimit() int {
	return c.concurrencyLimit
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
		"request_id", req.ID,
		"method", req.Method,
		"url", req.URL,
	)

	resp := c.executeRequest(ctx, req)
	c.recordTaskCompletion(req, resp)

	return c.publishResult(ctx, req, resp)
}

func (c *Consumer) parseTask(body []byte) (*protocol.Request, error) {
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

func (c *Consumer) executeRequest(ctx context.Context, req *protocol.Request) *protocol.Response {
	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		c.logger.Error("HTTP request failed",
			"request_id", req.ID,
			"error", err,
		)

		resp = &protocol.Response{
			RequestID: req.ID,
			EgressID:  c.egressID,
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   err.Error(),
				Retryable: true,
			},
		}
	}

	resp.EgressID = c.egressID

	return resp
}

func (c *Consumer) recordTaskCompletion(req *protocol.Request, resp *protocol.Response) {
	c.logger.Info("task completed",
		"request_id", req.ID,
		"status_code", resp.StatusCode,
		"has_error", resp.Error != nil,
	)
}

func (c *Consumer) publishResult(ctx context.Context, req *protocol.Request, resp *protocol.Response) error {
	if c.resultHandler != nil {
		err := c.resultHandler(ctx, resp, req.ReplyTo)
		if err != nil {
			c.logger.Error("failed to publish result",
				"request_id", req.ID,
				"error", err,
			)

			return err
		}
	}

	return nil
}

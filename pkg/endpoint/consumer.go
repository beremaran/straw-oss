package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/beremaran/straw/internal/endpoint/metrics"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
)

const DefaultMaxTaskAge = 60 * time.Second
const DefaultConcurrencyLimit = 25

// RequestExecutor defines the interface for executing a proxy request.
// This allows users to supply their own request executors or HTTP clients.
type RequestExecutor interface {
	Do(ctx context.Context, req *protocol.Request) (*protocol.Response, error)
}

type Consumer struct {
	broker           broker.MessageBroker
	httpClient       RequestExecutor
	secret           []byte
	endpointID       string
	taskSubject      string
	concurrencyLimit int
	semaphore        chan struct{}
	maxTaskAge       time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	resultHandler    func(ctx context.Context, resp *protocol.Response, replyTo string) error
	logger           *slog.Logger
	onTaskCompleted  func(TaskResult)
	mu               sync.Mutex
	subCtx           context.Context
	subCancel        context.CancelFunc
}

type TaskResult struct {
	RequestID     string
	StatusCode    int
	BytesSent     uint64
	BytesReceived uint64
	Latency       time.Duration
	HasError      bool
}

type Option func(*Consumer)

func WithConcurrencyLimit(n int) Option {
	return func(c *Consumer) {
		if n > 0 {
			c.concurrencyLimit = n
		}
	}
}

func WithMaxTaskAge(d time.Duration) Option {
	return func(c *Consumer) {
		if d > 0 {
			c.maxTaskAge = d
		}
	}
}

func WithResultHandler(h func(ctx context.Context, resp *protocol.Response, replyTo string) error) Option {
	return func(c *Consumer) {
		c.resultHandler = h
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(c *Consumer) {
		c.logger = logger
	}
}

func WithStatsCallback(h func(TaskResult)) Option {
	return func(c *Consumer) {
		c.onTaskCompleted = h
	}
}

func NewConsumer(
	b broker.MessageBroker,
	httpClient RequestExecutor,
	secret []byte,
	endpointID string,
	opts ...Option,
) *Consumer {
	c := &Consumer{
		broker:           b,
		httpClient:       httpClient,
		secret:           secret,
		endpointID:       endpointID,
		taskSubject:      "tasks." + endpointID + ".tasks",
		concurrencyLimit: DefaultConcurrencyLimit,
		maxTaskAge:       DefaultMaxTaskAge,
		logger:           slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	c.semaphore = make(chan struct{}, c.concurrencyLimit)

	return c
}

func (c *Consumer) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.logger.Info("starting consumer",
		"endpoint_id", c.endpointID,
		"subject", c.taskSubject,
		"concurrency_limit", c.concurrencyLimit,
		"max_task_age", c.maxTaskAge,
	)

	err := c.Resume(ctx)
	if err != nil {
		return err
	}

	<-c.ctx.Done()

	c.Drain()

	c.logger.Info("consumer stopped", "endpoint_id", c.endpointID)

	return nil
}

func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

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

		return err
	}

	return nil
}

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

type TaskError struct {
	Code    string
	Message string
}

func (e *TaskError) Error() string {
	return e.Code + ": " + e.Message
}

func (c *Consumer) TaskSubject() string {
	return c.taskSubject
}

func (c *Consumer) ConcurrencyLimit() int {
	return c.concurrencyLimit
}

func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
	metrics.TasksQueued.Inc()
	defer metrics.TasksQueued.Dec()

	select {
	case c.semaphore <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() { <-c.semaphore }()

		err := c.processTask(ctx, body)
		if err != nil {
			c.logger.Error("failed to process task",
				"error", err,
				"endpoint_id", c.endpointID,
			)
		}
	}()

	return nil
}

func (c *Consumer) processTask(ctx context.Context, body []byte) error {
	metrics.TasksInFlight.Inc()
	defer metrics.TasksInFlight.Dec()

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
	var signedTask protocol.SignedTask
	err := json.Unmarshal(body, &signedTask)
	if err != nil {
		c.logger.Warn("failed to unmarshal signed task",
			"error", err,
			"body_len", len(body),
		)

		return nil, &TaskError{
			Code:    protocol.ErrCodeInternalError,
			Message: "failed to unmarshal signed task: " + err.Error(),
		}
	}

	req, err := protocol.ValidateSignedTask(&signedTask, c.secret, c.maxTaskAge)
	if err != nil {
		c.logger.Warn("task validation failed",
			"error", err,
		)

		var valErr *protocol.ValidationError
		if errors.As(err, &valErr) {
			return nil, &TaskError{
				Code:    valErr.Code,
				Message: valErr.Message,
			}
		}

		return nil, &TaskError{
			Code:    protocol.ErrCodeInternalError,
			Message: "task validation failed: " + err.Error(),
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
			RequestID:  req.ID,
			EndpointID: c.endpointID,
			SessionID:  req.SessionID,
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   err.Error(),
				Retryable: true,
			},
		}
	}

	resp.EndpointID = c.endpointID

	return resp
}

func (c *Consumer) recordTaskCompletion(req *protocol.Request, resp *protocol.Response) {
	c.logger.Info("task completed",
		"request_id", req.ID,
		"status_code", resp.StatusCode,
		"has_error", resp.Error != nil,
	)

	status := "success"
	if resp.Error != nil {
		status = "failed"
		metrics.TasksFailed.WithLabelValues(resp.Error.Code).Inc()
	}
	metrics.TasksProcessed.WithLabelValues(status).Inc()

	bytesSent := req.EstimateWireSize()
	metrics.BytesSent.Add(float64(bytesSent))

	var bytesReceived uint64
	if resp != nil {
		bytesReceived = resp.EstimateWireSize()
		metrics.BytesReceived.Add(float64(bytesReceived))
	}

	if c.onTaskCompleted != nil {
		var latency time.Duration
		if resp.Timing != nil {
			latency = resp.Timing.Total
		}

		c.onTaskCompleted(TaskResult{
			RequestID:     req.ID,
			StatusCode:    resp.StatusCode,
			BytesSent:     bytesSent,
			BytesReceived: bytesReceived,
			Latency:       latency,
			HasError:      resp.Error != nil,
		})
	}
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

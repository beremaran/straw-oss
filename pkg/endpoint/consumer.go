//nolint:funcorder
package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	broker     broker.MessageBroker
	httpClient RequestExecutor
	secret     []byte
	endpointID string
	queueName  string

	concurrencyLimit int
	semaphore        chan struct{}

	maxTaskAge time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	resultHandler func(ctx context.Context, resp *protocol.Response, replyTo string) error

	logger *slog.Logger

	onTaskCompleted func(TaskResult)
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
		queueName:        "endpoint." + endpointID + ".tasks",
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
		"queue", c.queueName,
		"concurrency_limit", c.concurrencyLimit,
		"max_task_age", c.maxTaskAge,
	)

	err := c.broker.DeclareQueue(ctx, c.queueName)
	if err != nil {
		return fmt.Errorf("failed to declare task queue: %w", err)
	}

	err = c.broker.BindQueue(ctx, c.queueName, "tasks", c.queueName)
	if err != nil {
		return fmt.Errorf("failed to bind task queue to exchange: %w", err)
	}
	c.logger.Info("bound task queue to tasks exchange",
		"queue", c.queueName,
		"exchange", "tasks",
		"routing_key", c.queueName,
	)

	err = c.broker.Subscribe(ctx, c.queueName, c.handleMessage, broker.WithMaxAckPending(c.concurrencyLimit))
	if err != nil {
		return err
	}

	<-c.ctx.Done()

	c.wg.Wait()

	c.logger.Info("consumer stopped", "endpoint_id", c.endpointID)

	return nil
}

func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
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

//nolint:cyclop,funlen
func (c *Consumer) processTask(ctx context.Context, body []byte) error {
	metrics.TasksInFlight.Inc()
	defer metrics.TasksInFlight.Dec()

	var signedTask protocol.SignedTask
	err := json.Unmarshal(body, &signedTask)
	if err != nil {
		c.logger.Warn("failed to unmarshal signed task",
			"error", err,
			"body_len", len(body),
		)

		return &TaskError{
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
			return &TaskError{
				Code:    valErr.Code,
				Message: valErr.Message,
			}
		}

		return &TaskError{
			Code:    protocol.ErrCodeInternalError,
			Message: "task validation failed: " + err.Error(),
		}
	}

	c.logger.Info("processing task",
		"request_id", req.ID,
		"method", req.Method,
		"url", req.URL,
	)

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

func (c *Consumer) QueueName() string {
	return c.queueName
}

func (c *Consumer) ConcurrencyLimit() int {
	return c.concurrencyLimit
}

type TaskError struct {
	Code    string
	Message string
}

func (e *TaskError) Error() string {
	return e.Code + ": " + e.Message
}

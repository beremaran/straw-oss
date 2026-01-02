// Package consumer provides a RabbitMQ task consumer for the Endpoint worker.
// It receives tasks from the broker, validates signatures, and executes HTTP requests.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	endpointhttp "github.com/kwilabs/straw-proxy-server/internal/endpoint/http"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/metrics"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// DefaultMaxTaskAge is the default maximum age for a task to be considered valid.
const DefaultMaxTaskAge = 60 * time.Second

// DefaultConcurrencyLimit is the default number of concurrent tasks.
const DefaultConcurrencyLimit = 100

// Consumer consumes tasks from RabbitMQ and executes HTTP requests.
type Consumer struct {
	broker     broker.MessageBroker
	httpClient *endpointhttp.Client
	secret     []byte
	endpointID string
	queueName  string

	// Concurrency control
	concurrencyLimit int
	semaphore        chan struct{}

	// Task validation
	maxTaskAge time.Duration

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Result handler (to be wired in task 2.6)
	resultHandler func(ctx context.Context, resp *protocol.Response, replyTo string) error

	// Logger
	logger *slog.Logger

	// Stats callback
	onTaskCompleted func(TaskResult)
}

// TaskResult contains the results of a processed task.
type TaskResult struct {
	RequestID     string
	StatusCode    int
	BytesSent     uint64
	BytesReceived uint64
	Latency       time.Duration
	HasError      bool
}

// Option is a functional option for configuring the Consumer.
type Option func(*Consumer)

// WithConcurrencyLimit sets the maximum number of concurrent tasks.
func WithConcurrencyLimit(n int) Option {
	return func(c *Consumer) {
		if n > 0 {
			c.concurrencyLimit = n
		}
	}
}

// WithMaxTaskAge sets the maximum age for a task to be considered valid.
func WithMaxTaskAge(d time.Duration) Option {
	return func(c *Consumer) {
		if d > 0 {
			c.maxTaskAge = d
		}
	}
}

// WithResultHandler sets the callback for publishing results.
// This will be used by task 2.6 (Result Publisher).
func WithResultHandler(h func(ctx context.Context, resp *protocol.Response, replyTo string) error) Option {
	return func(c *Consumer) {
		c.resultHandler = h
	}
}

// WithLogger sets the logger for the consumer.
func WithLogger(logger *slog.Logger) Option {
	return func(c *Consumer) {
		c.logger = logger
	}
}

// WithStatsCallback sets the callback for task completion statistics.
func WithStatsCallback(h func(TaskResult)) Option {
	return func(c *Consumer) {
		c.onTaskCompleted = h
	}
}

// New creates a new Consumer.
func New(
	b broker.MessageBroker,
	httpClient *endpointhttp.Client,
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

	// Initialize semaphore for concurrency control
	c.semaphore = make(chan struct{}, c.concurrencyLimit)

	return c
}

// Start begins consuming tasks from the queue.
// This method blocks until the context is cancelled or an error occurs.
func (c *Consumer) Start(ctx context.Context) error {
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.logger.Info("starting consumer",
		"endpoint_id", c.endpointID,
		"queue", c.queueName,
		"concurrency_limit", c.concurrencyLimit,
		"max_task_age", c.maxTaskAge,
	)

	// Declare the queue before binding
	if err := c.broker.DeclareQueue(c.ctx, c.queueName); err != nil {
		return fmt.Errorf("failed to declare task queue: %w", err)
	}

	// Bind the queue to the tasks exchange
	// The relay server publishes to "tasks" exchange with routing key = queue name
	if err := c.broker.BindQueue(c.ctx, c.queueName, "tasks", c.queueName); err != nil {
		return fmt.Errorf("failed to bind task queue to exchange: %w", err)
	}
	c.logger.Info("bound task queue to tasks exchange",
		"queue", c.queueName,
		"exchange", "tasks",
		"routing_key", c.queueName,
	)

	// Subscribe to the task queue
	err := c.broker.Subscribe(c.ctx, c.queueName, c.handleMessage)
	if err != nil {
		return err
	}

	// Wait for context cancellation
	<-c.ctx.Done()

	// Wait for in-flight tasks to complete
	c.wg.Wait()

	c.logger.Info("consumer stopped", "endpoint_id", c.endpointID)
	return nil
}

// Stop gracefully stops the consumer.
func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// handleMessage is the broker handler for incoming messages.
func (c *Consumer) handleMessage(ctx context.Context, body []byte) error {
	// Acquire semaphore slot for concurrency control
	select {
	case c.semaphore <- struct{}{}:
		// Got a slot
	case <-ctx.Done():
		return ctx.Err()
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() { <-c.semaphore }() // Release semaphore slot

		if err := c.processTask(ctx, body); err != nil {
			c.logger.Error("failed to process task",
				"error", err,
				"endpoint_id", c.endpointID,
			)
		}
	}()

	return nil
}

// processTask processes a single task.
func (c *Consumer) processTask(ctx context.Context, body []byte) error {
	metrics.TasksInFlight.Inc()
	defer metrics.TasksInFlight.Dec()

	// Deserialize the signed task
	var signedTask protocol.SignedTask
	if err := json.Unmarshal(body, &signedTask); err != nil {
		c.logger.Warn("failed to unmarshal signed task",
			"error", err,
			"body_len", len(body),
		)
		return &TaskError{
			Code:    protocol.ErrCodeInternalError,
			Message: "failed to unmarshal signed task: " + err.Error(),
		}
	}

	// Validate signature and timestamp
	req, err := protocol.ValidateSignedTask(&signedTask, c.secret, c.maxTaskAge)
	if err != nil {
		c.logger.Warn("task validation failed",
			"error", err,
		)
		// Check if it's a validation error to extract the code
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

	// Execute the HTTP request
	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		c.logger.Error("HTTP request failed",
			"request_id", req.ID,
			"error", err,
		)
		// Build error response
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

	// Set endpoint ID on response
	resp.EndpointID = c.endpointID

	c.logger.Info("task completed",
		"request_id", req.ID,
		"status_code", resp.StatusCode,
		"has_error", resp.Error != nil,
	)

	// Record task processed metric
	status := "success"
	if resp.Error != nil {
		status = "failed"
		metrics.TasksFailed.WithLabelValues(resp.Error.Code).Inc()
	}
	metrics.TasksProcessed.WithLabelValues(status).Inc()

	// Track bandwidth
	bytesSent := req.EstimateWireSize()
	metrics.BytesSent.Add(float64(bytesSent))

	var bytesReceived uint64
	if resp != nil {
		bytesReceived = resp.EstimateWireSize()
		metrics.BytesReceived.Add(float64(bytesReceived))
	}

	// Notify stats callback
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

	// Publish result if handler is configured
	if c.resultHandler != nil {
		if err := c.resultHandler(ctx, resp, req.ReplyTo); err != nil {
			c.logger.Error("failed to publish result",
				"request_id", req.ID,
				"error", err,
			)
			return err
		}
	}

	return nil
}

// QueueName returns the queue name that this consumer subscribes to.
func (c *Consumer) QueueName() string {
	return c.queueName
}

// ConcurrencyLimit returns the current concurrency limit.
func (c *Consumer) ConcurrencyLimit() int {
	return c.concurrencyLimit
}

// TaskError represents an error during task processing.
type TaskError struct {
	Code    string
	Message string
}

func (e *TaskError) Error() string {
	return e.Code + ": " + e.Message
}

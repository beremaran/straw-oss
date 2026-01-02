// Package integration provides testcontainer-based infrastructure for integration testing.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/heartbeat"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// MockEndpointConfig configures the mock endpoint behavior.
type MockEndpointConfig struct {
	// EndpointID is the identifier for this mock endpoint.
	EndpointID string
	// QueueName to consume from (defaults to endpoint.<id>.tasks).
	QueueName string
	// Secret is the HMAC secret for signature validation.
	Secret []byte
	// TargetURL is the base URL of the mock target server to forward requests to.
	// If empty, the endpoint will generate synthetic responses.
	TargetURL string
	// Tags are the endpoint tags to include in heartbeats.
	Tags []string
	// HeartbeatInterval is the interval between heartbeats (default: 1s for tests).
	HeartbeatInterval time.Duration
}

// MockEndpointResponse configures a controlled response from the mock endpoint.
type MockEndpointResponse struct {
	// StatusCode to return.
	StatusCode int
	// Headers to include.
	Headers protocol.HeaderMap
	// Body to return.
	Body []byte
	// Error to return instead of a successful response.
	Error *protocol.ErrorInfo
	// Delay before responding.
	Delay time.Duration
}

// EndpointRequestRecord stores a received request on the mock endpoint.
type EndpointRequestRecord struct {
	RequestID   string
	Method      string
	URL         string
	Fingerprint string
	Time        time.Time
}

// MockEndpoint simulates an Endpoint worker that consumes tasks from RabbitMQ.
type MockEndpoint struct {
	config MockEndpointConfig
	broker broker.MessageBroker
	logger *slog.Logger

	mu              sync.RWMutex
	response        *MockEndpointResponse
	requests        []EndpointRequestRecord
	failureCount    int32 // number of failures before success
	failuresLeft    int32
	httpClient      *http.Client
	targetURL       string
	running         atomic.Bool
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
	responseHandler func(*protocol.Request) *MockEndpointResponse

	// heartbeatSender sends periodic heartbeats to register the endpoint
	heartbeatSender *heartbeat.Sender
}

// NewMockEndpoint creates a new mock endpoint.
func NewMockEndpoint(b broker.MessageBroker, config MockEndpointConfig) *MockEndpoint {
	if config.QueueName == "" {
		config.QueueName = "endpoint." + config.EndpointID + ".tasks"
	}
	// Default heartbeat interval to 1 second for faster test feedback
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 1 * time.Second
	}
	return &MockEndpoint{
		config:     config,
		broker:     b,
		logger:     slog.Default(),
		httpClient: &http.Client{Timeout: 30 * time.Second},
		targetURL:  config.TargetURL,
	}
}

// Start begins consuming tasks from RabbitMQ and sending heartbeats.
func (m *MockEndpoint) Start(ctx context.Context) error {
	if m.running.Load() {
		return fmt.Errorf("mock endpoint already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	m.cancelFunc = cancel
	m.running.Store(true)

	m.logger.Info("starting mock endpoint",
		"endpoint_id", m.config.EndpointID,
		"queue", m.config.QueueName,
		"tags", m.config.Tags,
	)

	// Start heartbeat sender to register endpoint in Redis
	m.heartbeatSender = heartbeat.New(
		m.broker,
		m.config.EndpointID,
		heartbeat.WithTags(m.config.Tags),
		heartbeat.WithInterval(m.config.HeartbeatInterval),
	)
	m.heartbeatSender.Start(ctx)

	// Start consuming tasks
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		// Ensure queue exists and is bound to tasks exchange
		// We do this in the goroutine or before? Subscription is typically blocking?
		// Subscribe is non-blocking (returns error or nil).
		// But Subscribe in broker/rabbitmq.go spawns a goroutine.
		// So we should do declared/bind here before Subscribe.
		// Note: broker methods use locks, so it's safe.

		if err := m.broker.DeclareQueue(ctx, m.config.QueueName); err != nil {
			m.logger.Error("failed to declare queue", "error", err)
			return
		}

		if err := m.broker.BindQueue(ctx, m.config.QueueName, "tasks", m.config.QueueName); err != nil {
			m.logger.Error("failed to bind queue", "error", err)
			return
		}

		err := m.broker.Subscribe(ctx, m.config.QueueName, m.handleMessage)
		if err != nil && ctx.Err() == nil {
			m.logger.Error("mock endpoint subscription error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the mock endpoint.
func (m *MockEndpoint) Stop() {
	// Stop heartbeat sender
	if m.heartbeatSender != nil {
		m.heartbeatSender.Stop()
	}
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	m.wg.Wait()
	m.running.Store(false)
	m.logger.Info("mock endpoint stopped", "endpoint_id", m.config.EndpointID)
}

// SetResponse sets the response to return for all requests.
func (m *MockEndpoint) SetResponse(resp *MockEndpointResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.response = resp
}

// SetResponseHandler sets a custom handler function for generating responses.
func (m *MockEndpoint) SetResponseHandler(handler func(*protocol.Request) *MockEndpointResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseHandler = handler
}

// SetFailures configures the endpoint to fail the first N requests.
func (m *MockEndpoint) SetFailures(count int) {
	atomic.StoreInt32(&m.failureCount, int32(count))
	atomic.StoreInt32(&m.failuresLeft, int32(count))
}

// SetTargetURL sets the target URL for forwarding requests.
func (m *MockEndpoint) SetTargetURL(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targetURL = url
}

// GetRequests returns all recorded requests.
func (m *MockEndpoint) GetRequests() []EndpointRequestRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]EndpointRequestRecord, len(m.requests))
	copy(result, m.requests)
	return result
}

// ClearRequests clears all recorded requests.
func (m *MockEndpoint) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
}

// RequestCount returns the number of recorded requests.
func (m *MockEndpoint) RequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.requests)
}

// EndpointID returns the endpoint ID.
func (m *MockEndpoint) EndpointID() string {
	return m.config.EndpointID
}

func (m *MockEndpoint) handleMessage(ctx context.Context, body []byte) error {
	// Parse the signed task
	var signedTask protocol.SignedTask
	if err := json.Unmarshal(body, &signedTask); err != nil {
		m.logger.Error("failed to unmarshal signed task", "error", err)
		return err
	}

	// Validate signature
	req, err := protocol.ValidateSignedTask(&signedTask, m.config.Secret, 60*time.Second)
	if err != nil {
		m.logger.Error("failed to validate signed task", "error", err)
		return err
	}

	// Record the request
	m.mu.Lock()
	m.requests = append(m.requests, EndpointRequestRecord{
		RequestID:   req.ID,
		Method:      req.Method,
		URL:         req.URL,
		Fingerprint: req.Fingerprint,
		Time:        time.Now(),
	})
	m.mu.Unlock()

	m.logger.Info("mock endpoint received request",
		"request_id", req.ID,
		"method", req.Method,
		"url", req.URL,
	)

	// Build response
	resp := m.buildResponse(ctx, req)

	// Publish response to the response queue
	respQueueName := req.ReplyTo
	if respQueueName == "" {
		respQueueName = fmt.Sprintf("results.%s", req.ID)
	}

	// Convert to result message format that matches orchestrator expectations
	resultMsg := resp.ToResultMessage(m.config.EndpointID, "", req.ID)

	respBody, err := json.Marshal(resultMsg)
	if err != nil {
		m.logger.Error("failed to marshal response", "error", err)
		return err
	}

	if err := m.broker.Publish(ctx, "", respQueueName, respBody); err != nil {
		m.logger.Error("failed to publish response", "error", err)
		return err
	}

	m.logger.Info("mock endpoint published response",
		"request_id", req.ID,
		"status_code", resp.StatusCode,
		"queue", respQueueName,
	)

	return nil
}

func (m *MockEndpoint) buildResponse(ctx context.Context, req *protocol.Request) *MockEndpointResponse {
	// Check if we should simulate failure
	if failuresLeft := atomic.AddInt32(&m.failuresLeft, -1); failuresLeft >= 0 {
		return &MockEndpointResponse{
			StatusCode: 503,
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   "simulated endpoint failure",
				Retryable: true,
			},
		}
	}

	m.mu.RLock()
	response := m.response
	handler := m.responseHandler
	targetURL := m.targetURL
	m.mu.RUnlock()

	// Use custom handler if set
	if handler != nil {
		return handler(req)
	}

	// Use configured response if set
	if response != nil {
		if response.Delay > 0 {
			time.Sleep(response.Delay)
		}
		return response
	}

	// Forward to target URL if configured
	if targetURL != "" {
		return m.forwardRequest(ctx, req, targetURL)
	}

	// Default: return success
	return &MockEndpointResponse{
		StatusCode: http.StatusOK,
		Headers: protocol.HeaderMap{
			{Key: "Content-Type", Value: "text/plain"},
		},
		Body: []byte(fmt.Sprintf("mock response for %s", req.URL)),
	}
}

func (m *MockEndpoint) forwardRequest(ctx context.Context, req *protocol.Request, targetBase string) *MockEndpointResponse {
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
	if err != nil {
		return &MockEndpointResponse{
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   fmt.Sprintf("failed to create request: %v", err),
				Retryable: false,
			},
		}
	}

	// Copy headers
	for _, h := range req.Headers {
		httpReq.Header.Set(h.Key, h.Value)
	}

	// Execute request
	resp, err := m.httpClient.Do(httpReq)
	if err != nil {
		return &MockEndpointResponse{
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   fmt.Sprintf("request failed: %v", err),
				Retryable: true,
			},
		}
	}
	defer resp.Body.Close()

	// Read body
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))

	// Build response headers
	headers := protocol.HeaderMap{}
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers = append(headers, protocol.Header{Key: k, Value: v[0]})
		}
	}

	return &MockEndpointResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}
}

// ResultMessage matches the format expected by the orchestrator consumer.
type MockResultMessage struct {
	RequestID      string               `json:"request_id"`
	StatusCode     int                  `json:"status_code"`
	Headers        protocol.HeaderMap   `json:"headers"`
	CompressedBody []byte               `json:"body"`
	BodyCompressed bool                 `json:"body_compressed"`
	EndpointID     string               `json:"endpoint_id"`
	SessionID      string               `json:"session_id"`
	Timing         *protocol.TimingInfo `json:"timing,omitempty"`
	Error          *protocol.ErrorInfo  `json:"error,omitempty"`
}

// MarshalJSON custom marshals the response for the broker.
func (r *MockEndpointResponse) ToResultMessage(endpointID, sessionID, requestID string) *MockResultMessage {
	return &MockResultMessage{
		RequestID:      requestID,
		StatusCode:     r.StatusCode,
		Headers:        r.Headers,
		CompressedBody: r.Body,
		BodyCompressed: false,
		EndpointID:     endpointID,
		SessionID:      sessionID,
		Timing: &protocol.TimingInfo{
			Total: 50 * time.Millisecond, // Simulated timing
		},
		Error: r.Error,
	}
}

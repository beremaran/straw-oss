// Package integration provides mock utilities for integration testing.
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/endpoint"
	"github.com/beremaran/straw/pkg/protocol"
)

// ErrMockEndpointAlreadyRunning is returned when attempting to start a
// MockEndpoint that is already running.
var ErrMockEndpointAlreadyRunning = errors.New("mock endpoint already running")

const (
	mockHTTPTimeout        = 30 * time.Second
	mockValidationMaxAge   = 60 * time.Second
	mockFailureStatusCode  = 503
	mockMaxResponseBody    = 10 * 1024 * 1024
	mockResultTotalLatency = 50 * time.Millisecond

	mockContentType = "Content-Type"
)

// MockEndpointConfig holds configuration for a MockEndpoint.
type MockEndpointConfig struct {
	EndpointID        string
	TaskSubject       string
	Secret            []byte
	TargetURL         string
	Tags              []string
	HeartbeatInterval time.Duration
}

// MockEndpointResponse defines the shape of a response returned by a
// MockEndpoint.
type MockEndpointResponse struct {
	StatusCode int
	Headers    protocol.HeaderMap
	Body       []byte
	Error      *protocol.ErrorInfo
	Delay      time.Duration
}

// EndpointRequestRecord stores metadata about a request received by a
// MockEndpoint.
type EndpointRequestRecord struct {
	RequestID   string
	Method      string
	URL         string
	Fingerprint string
	Time        time.Time
}

// MockEndpoint is a test double that mimics an endpoint server.
type MockEndpoint struct {
	config          MockEndpointConfig
	broker          broker.MessageBroker
	logger          *slog.Logger
	mu              sync.RWMutex
	response        *MockEndpointResponse
	requests        []EndpointRequestRecord
	failureCount    atomic.Int32
	failuresLeft    atomic.Int32
	httpClient      *http.Client
	targetURL       string
	running         atomic.Bool
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
	responseHandler func(*protocol.Request) *MockEndpointResponse
	heartbeatSender *endpoint.HeartbeatSender
}

// NewMockEndpoint creates a new MockEndpoint instance.
func NewMockEndpoint(b broker.MessageBroker, config MockEndpointConfig) *MockEndpoint {
	if config.TaskSubject == "" {
		config.TaskSubject = "tasks." + config.EndpointID + ".tasks"
	}

	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 1 * time.Second
	}

	return &MockEndpoint{
		config:     config,
		broker:     b,
		logger:     slog.Default(),
		httpClient: &http.Client{Timeout: mockHTTPTimeout},
		targetURL:  config.TargetURL,
	}
}

// Start begins the mock endpoint, subscribing to its task subject and
// starting the heartbeat sender.
func (m *MockEndpoint) Start(ctx context.Context) error {
	if m.running.Load() {
		return ErrMockEndpointAlreadyRunning
	}

	ctx, cancel := context.WithCancel(ctx)
	m.cancelFunc = cancel
	m.running.Store(true)

	m.logger.Info("starting mock endpoint",
		"endpoint_id", m.config.EndpointID,
		"subject", m.config.TaskSubject,
		"tags", m.config.Tags,
	)

	m.heartbeatSender = endpoint.NewHeartbeatSender(
		m.broker,
		m.config.EndpointID,
		endpoint.WithHeartbeatTags(m.config.Tags),
		endpoint.WithHeartbeatInterval(m.config.HeartbeatInterval),
	)
	m.heartbeatSender.Start(ctx)

	m.wg.Go(func() {
		err := m.broker.Subscribe(ctx, m.config.TaskSubject, m.handleMessage)
		if err != nil && ctx.Err() == nil {
			m.logger.Error("mock endpoint subscription error", "error", err)
		}
	})

	return nil
}

// Stop halts the mock endpoint and waits for in-flight operations to finish.
func (m *MockEndpoint) Stop() {
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

// SetResponse configures a static response for the mock endpoint.
func (m *MockEndpoint) SetResponse(resp *MockEndpointResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.response = resp
}

// SetResponseHandler configures a dynamic response function for the mock
// endpoint.
func (m *MockEndpoint) SetResponseHandler(handler func(*protocol.Request) *MockEndpointResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.responseHandler = handler
}

// SetFailures configures the mock endpoint to return a failure response
// for the given number of requests.
func (m *MockEndpoint) SetFailures(count int) {
	if count > math.MaxInt32 {
		count = math.MaxInt32
	} else if count < math.MinInt32 {
		count = math.MinInt32
	}

	m.failureCount.Store(int32(count))
	m.failuresLeft.Store(int32(count))
}

// SetTargetURL configures the URL to forward requests to.
func (m *MockEndpoint) SetTargetURL(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.targetURL = url
}

// GetRequests returns a copy of all recorded requests.
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

// EndpointID returns the endpoint ID configured for this mock endpoint.
func (m *MockEndpoint) EndpointID() string {
	return m.config.EndpointID
}

func (m *MockEndpoint) handleMessage(ctx context.Context, body []byte) error {
	req, err := m.parseRequest(body)
	if err != nil {
		return err
	}

	m.recordRequest(req)
	m.logger.Info("mock endpoint received request",
		"request_id", req.ID,
		"method", req.Method,
		"url", req.URL,
	)

	return m.publishResponse(ctx, req, m.buildResponse(ctx, req))
}

func (m *MockEndpoint) parseRequest(body []byte) (*protocol.Request, error) {
	var signedTask protocol.SignedTask

	err := json.Unmarshal(body, &signedTask)
	if err != nil {
		m.logger.Error("failed to unmarshal signed task", "error", err)

		return nil, fmt.Errorf("unmarshal signed task: %w", err)
	}

	req, err := protocol.ValidateSignedTask(&signedTask, m.config.Secret, mockValidationMaxAge)
	if err != nil {
		m.logger.Error("failed to validate signed task", "error", err)

		return nil, fmt.Errorf("validate signed task: %w", err)
	}

	return req, nil
}

func (m *MockEndpoint) recordRequest(req *protocol.Request) {
	m.mu.Lock()
	m.requests = append(m.requests, EndpointRequestRecord{
		RequestID:   req.ID,
		Method:      req.Method,
		URL:         req.URL,
		Fingerprint: req.Fingerprint,
		Time:        time.Now(),
	})
	m.mu.Unlock()
}

func (m *MockEndpoint) publishResponse(ctx context.Context, req *protocol.Request, resp *MockEndpointResponse) error {
	respSubjectName := responseSubjectName(req)
	resultMsg := resp.ToResultMessage(m.config.EndpointID, "", req.ID)

	respBody, err := json.Marshal(resultMsg)
	if err != nil {
		m.logger.Error("failed to marshal response", "error", err)

		return fmt.Errorf("marshal response: %w", err)
	}

	err = m.broker.Publish(ctx, respSubjectName, respBody)
	if err != nil {
		m.logger.Error("failed to publish response", "error", err)

		return fmt.Errorf("publish response: %w", err)
	}

	m.logger.Info("mock endpoint published response",
		"request_id", req.ID,
		"status_code", resp.StatusCode,
		"subject", respSubjectName,
	)

	return nil
}

func responseSubjectName(req *protocol.Request) string {
	if req.ReplyTo != "" {
		return req.ReplyTo
	}

	return "results." + req.ID
}

func (m *MockEndpoint) buildResponse(ctx context.Context, req *protocol.Request) *MockEndpointResponse {
	if failuresLeft := m.failuresLeft.Add(-1); failuresLeft >= 0 {
		return &MockEndpointResponse{
			StatusCode: mockFailureStatusCode,
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

	if handler != nil {
		return handler(req)
	}

	if response != nil {
		if response.Delay > 0 {
			time.Sleep(response.Delay)
		}

		return response
	}

	if targetURL != "" {
		return m.forwardRequest(ctx, req, targetURL)
	}

	return &MockEndpointResponse{
		StatusCode: http.StatusOK,
		Headers: protocol.HeaderMap{
			{Key: mockContentType, Value: textPlain},
		},
		Body: []byte("mock response for " + req.URL),
	}
}

func (m *MockEndpoint) forwardRequest(ctx context.Context, req *protocol.Request, _ string) *MockEndpointResponse {
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

	for _, h := range req.Headers {
		httpReq.Header.Set(h.Key, h.Value)
	}

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
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, mockMaxResponseBody))

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

// MockResultMessage is the response payload published by a MockEndpoint.
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

// ToResultMessage converts a MockEndpointResponse into a MockResultMessage
// for publishing to the broker.
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
			Total: mockResultTotalLatency,
		},
		Error: r.Error,
	}
}

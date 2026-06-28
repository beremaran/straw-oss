package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/endpoint"
	"github.com/beremaran/straw/pkg/protocol"
)

var ErrMockEndpointAlreadyRunning = errors.New("mock endpoint already running")

type MockEndpointConfig struct {
	EndpointID string

	QueueName string

	Secret []byte

	TargetURL string

	Tags []string

	HeartbeatInterval time.Duration
}

type MockEndpointResponse struct {
	StatusCode int

	Headers protocol.HeaderMap

	Body []byte

	Error *protocol.ErrorInfo

	Delay time.Duration
}

type EndpointRequestRecord struct {
	RequestID   string
	Method      string
	URL         string
	Fingerprint string
	Time        time.Time
}

type MockEndpoint struct {
	config MockEndpointConfig
	broker broker.MessageBroker
	logger *slog.Logger

	mu              sync.RWMutex
	response        *MockEndpointResponse
	requests        []EndpointRequestRecord
	failureCount    int32
	failuresLeft    int32
	httpClient      *http.Client
	targetURL       string
	running         atomic.Bool
	cancelFunc      context.CancelFunc
	wg              sync.WaitGroup
	responseHandler func(*protocol.Request) *MockEndpointResponse

	heartbeatSender *endpoint.HeartbeatSender
}

func NewMockEndpoint(b broker.MessageBroker, config MockEndpointConfig) *MockEndpoint {
	if config.QueueName == "" {
		config.QueueName = "endpoint." + config.EndpointID + ".tasks"
	}

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

func (m *MockEndpoint) Start(ctx context.Context) error {
	if m.running.Load() {
		return ErrMockEndpointAlreadyRunning
	}

	ctx, cancel := context.WithCancel(ctx)
	m.cancelFunc = cancel
	m.running.Store(true)

	m.logger.Info("starting mock endpoint",
		"endpoint_id", m.config.EndpointID,
		"queue", m.config.QueueName,
		"tags", m.config.Tags,
	)

	m.heartbeatSender = endpoint.NewHeartbeatSender(
		m.broker,
		m.config.EndpointID,
		endpoint.WithHeartbeatTags(m.config.Tags),
		endpoint.WithHeartbeatInterval(m.config.HeartbeatInterval),
	)
	m.heartbeatSender.Start(ctx)

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		err := m.broker.DeclareQueue(ctx, m.config.QueueName)
		if err != nil {
			m.logger.Error("failed to declare queue", "error", err)

			return
		}

		err = m.broker.BindQueue(ctx, m.config.QueueName, "tasks", m.config.QueueName)
		if err != nil {
			m.logger.Error("failed to bind queue", "error", err)

			return
		}

		err = m.broker.Subscribe(ctx, m.config.QueueName, m.handleMessage)
		if err != nil && ctx.Err() == nil {
			m.logger.Error("mock endpoint subscription error", "error", err)
		}
	}()

	return nil
}

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

func (m *MockEndpoint) SetResponse(resp *MockEndpointResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.response = resp
}

func (m *MockEndpoint) SetResponseHandler(handler func(*protocol.Request) *MockEndpointResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseHandler = handler
}

func (m *MockEndpoint) SetFailures(count int) {
	atomic.StoreInt32(&m.failureCount, int32(count))
	atomic.StoreInt32(&m.failuresLeft, int32(count))
}

func (m *MockEndpoint) SetTargetURL(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targetURL = url
}

func (m *MockEndpoint) GetRequests() []EndpointRequestRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]EndpointRequestRecord, len(m.requests))
	copy(result, m.requests)

	return result
}

func (m *MockEndpoint) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = nil
}

func (m *MockEndpoint) RequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.requests)
}

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

		return nil, err
	}

	req, err := protocol.ValidateSignedTask(&signedTask, m.config.Secret, 60*time.Second)
	if err != nil {
		m.logger.Error("failed to validate signed task", "error", err)

		return nil, err
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
	respQueueName := responseQueueName(req)
	resultMsg := resp.ToResultMessage(m.config.EndpointID, "", req.ID)

	respBody, err := json.Marshal(resultMsg)
	if err != nil {
		m.logger.Error("failed to marshal response", "error", err)

		return err
	}

	err = m.broker.Publish(ctx, "", respQueueName, respBody)
	if err != nil {
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

func responseQueueName(req *protocol.Request) string {
	if req.ReplyTo != "" {
		return req.ReplyTo
	}

	return fmt.Sprintf("results.%s", req.ID)
}

func (m *MockEndpoint) buildResponse(ctx context.Context, req *protocol.Request) *MockEndpointResponse {
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
			{Key: "Content-Type", Value: "text/plain"},
		},
		Body: []byte(fmt.Sprintf("mock response for %s", req.URL)),
	}
}

func (m *MockEndpoint) forwardRequest(ctx context.Context, req *protocol.Request, targetBase string) *MockEndpointResponse {
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

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))

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
			Total: 50 * time.Millisecond,
		},
		Error: r.Error,
	}
}

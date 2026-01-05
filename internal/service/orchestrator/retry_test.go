package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// retryMockBroker handles message simulation for RetryExecutor tests.
type retryMockBroker struct {
	mu            sync.Mutex
	subscriptions map[string]broker.Handler
	responseFunc  func(routingKey string, body []byte) []byte
}

func newRetryMockBroker() *retryMockBroker {
	return &retryMockBroker{
		subscriptions: make(map[string]broker.Handler),
	}
}

func (m *retryMockBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	m.mu.Lock()
	handler := m.subscriptions["temp-reply-queue"]
	respFunc := m.responseFunc
	m.mu.Unlock()

	if respFunc != nil {
		responseBody := respFunc(routingKey, body)
		// Simulate async response
		go func() {
			if handler != nil {
				handler(ctx, responseBody)
			}
		}()
	}
	return nil
}

func (m *retryMockBroker) Subscribe(ctx context.Context, queue string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	return nil
}

func (m *retryMockBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
	m.mu.Lock()
	m.subscriptions["temp-reply-queue"] = handler
	m.mu.Unlock()
	return nil
}

func (m *retryMockBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (m *retryMockBroker) Close() error                                                 { return nil }
func (m *retryMockBroker) DeclareExchange(ctx context.Context, name, kind string) error { return nil }
func (m *retryMockBroker) DeclareQueue(ctx context.Context, name string) error          { return nil }
func (m *retryMockBroker) BindQueue(ctx context.Context, queue, exchange, routingKey string) error {
	return nil
}

func (m *retryMockBroker) IsConnected() bool {
	return true
}

func (m *retryMockBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	return 0, nil
}

type mockPoolManager struct {
	endpoints   map[int][]string
	poolConfigs map[int]*domain.EndpointPool
}

func newMockPoolManager() *mockPoolManager {
	return &mockPoolManager{
		endpoints:   make(map[int][]string),
		poolConfigs: make(map[int]*domain.EndpointPool),
	}
}

func (m *mockPoolManager) GetEndpointFromPool(ctx context.Context, rule *domain.RoutingRule, poolTier int, exclude []string) (string, error) {
	endpoints, ok := m.endpoints[poolTier]
	if !ok || len(endpoints) == 0 {
		return "", errors.New("no endpoints in pool")
	}
	for _, ep := range endpoints {
		excluded := false
		for _, ex := range exclude {
			if ep == ex {
				excluded = true
				break
			}
		}
		if !excluded {
			return ep, nil
		}
	}
	return "", errors.New("all endpoints excluded")
}

func (m *mockPoolManager) GetPoolConfig(rule *domain.RoutingRule, poolTier int) *domain.EndpointPool {
	return m.poolConfigs[poolTier]
}

func TestRetryExecutor_getPoolTiers(t *testing.T) {
	executor := &RetryExecutor{}

	t.Run("single pool", func(t *testing.T) {
		rule := &domain.RoutingRule{
			EndpointPools: []domain.EndpointPool{{Tier: 1}},
		}
		tiers := executor.getPoolTiers(rule)
		if len(tiers) != 1 || tiers[0] != 1 {
			t.Error("failed getPoolTiers for single pool")
		}
	})

	t.Run("multiple pools unsorted", func(t *testing.T) {
		rule := &domain.RoutingRule{
			EndpointPools: []domain.EndpointPool{
				{Tier: 3},
				{Tier: 1},
				{Tier: 2},
			},
		}
		tiers := executor.getPoolTiers(rule)
		if len(tiers) != 3 || tiers[0] != 1 || tiers[1] != 2 || tiers[2] != 3 {
			t.Error("failed getPoolTiers for unsorted pools")
		}
	})

	t.Run("empty pools", func(t *testing.T) {
		rule := &domain.RoutingRule{
			EndpointPools: []domain.EndpointPool{},
		}
		tiers := executor.getPoolTiers(rule)
		if len(tiers) != 0 {
			t.Error("expected empty tiers for empty pools")
		}
	})
}

func TestRetryExecutor_getMaxRetries(t *testing.T) {
	executor := &RetryExecutor{}

	// Test default when pool config is nil
	maxRetries := executor.getMaxRetries(nil, 1)
	if maxRetries != DefaultMaxRetries {
		t.Errorf("expected default max retries %d, got %d", DefaultMaxRetries, maxRetries)
	}

	// Test pool config with custom max retries
	poolConfig := &domain.EndpointPool{MaxRetries: 5}
	maxRetries = executor.getMaxRetries(poolConfig, 1)
	if maxRetries != 5 {
		t.Errorf("expected max retries 5, got %d", maxRetries)
	}

	// Test last exit pool (tier >= 4) gets only 1 attempt
	maxRetries = executor.getMaxRetries(nil, 4)
	if maxRetries != DefaultLastExitRetries {
		t.Errorf("expected last exit retries %d, got %d", DefaultLastExitRetries, maxRetries)
	}

	maxRetries = executor.getMaxRetries(nil, 5)
	if maxRetries != DefaultLastExitRetries {
		t.Errorf("expected last exit retries %d, got %d", DefaultLastExitRetries, maxRetries)
	}
}

func TestRetryExecutor_calculateBackoff(t *testing.T) {
	executor := &RetryExecutor{
		baseBackoff:   100 * time.Millisecond,
		maxBackoff:    5 * time.Second,
		backoffFactor: 2.0,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, 1600 * time.Millisecond},
		{6, 3200 * time.Millisecond},
		{7, 5000 * time.Millisecond}, // Should cap at maxBackoff (5s)
		{8, 5000 * time.Millisecond}, // Capped
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			backoff := executor.calculateBackoff(tt.attempt)
			if backoff != tt.expected {
				t.Errorf("attempt %d: expected backoff %v, got %v", tt.attempt, tt.expected, backoff)
			}
		})
	}
}

func TestRetryExecutor_getFailureMessage(t *testing.T) {
	executor := &RetryExecutor{}

	t.Run("with error info", func(t *testing.T) {
		result := &ResultMessage{
			Error: &protocol.ErrorInfo{
				Message: "connection refused",
			},
		}
		msg := executor.getFailureMessage(result)
		if msg != "connection refused" {
			t.Errorf("expected 'connection refused', got %q", msg)
		}
	})

	t.Run("without error info", func(t *testing.T) {
		result := &ResultMessage{
			StatusCode: 500,
		}
		msg := executor.getFailureMessage(result)
		if msg != "HTTP 500" {
			t.Errorf("expected 'HTTP 500', got %q", msg)
		}
	})
}

func TestRetryExecutor_parseResult(t *testing.T) {
	executor := &RetryExecutor{}

	t.Run("valid result", func(t *testing.T) {
		result := ResultMessage{
			RequestID:  "test-123",
			StatusCode: 200,
		}
		body, _ := json.Marshal(result)
		parsed, err := executor.parseResult(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed.RequestID != "test-123" {
			t.Errorf("expected request ID 'test-123', got %q", parsed.RequestID)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := executor.parseResult([]byte("invalid json"))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestRetryResult(t *testing.T) {
	result := &RetryResult{
		Success:      true,
		FinalPool:    1,
		TotalRetries: 2,
		AttemptErrors: []AttemptError{
			{
				Pool:          1,
				Attempt:       1,
				EndpointID:    "ep-1",
				Failure:       FailureTimeout,
				FailureString: "timeout",
				Message:       "request timed out",
			},
		},
	}

	if !result.Success {
		t.Error("expected success to be true")
	}

	if result.FinalPool != 1 {
		t.Errorf("expected final pool 1, got %d", result.FinalPool)
	}

	if result.TotalRetries != 2 {
		t.Errorf("expected total retries 2, got %d", result.TotalRetries)
	}

	if len(result.AttemptErrors) != 1 {
		t.Errorf("expected 1 attempt error, got %d", len(result.AttemptErrors))
	}
}

func TestAttemptError(t *testing.T) {
	err := AttemptError{
		Pool:          2,
		Attempt:       3,
		EndpointID:    "ep-2",
		Failure:       FailureConnection,
		FailureString: "connection",
		Message:       "connection reset",
		Duration:      500 * time.Millisecond,
	}

	if err.Pool != 2 {
		t.Errorf("expected pool 2, got %d", err.Pool)
	}

	if err.Attempt != 3 {
		t.Errorf("expected attempt 3, got %d", err.Attempt)
	}

	if err.EndpointID != "ep-2" {
		t.Errorf("expected endpoint ID 'ep-2', got %q", err.EndpointID)
	}

	if err.Failure != FailureConnection {
		t.Errorf("expected failure type FailureConnection, got %v", err.Failure)
	}

	if err.FailureString != "connection" {
		t.Errorf("expected failure string 'connection', got %q", err.FailureString)
	}

	if err.Message != "connection reset" {
		t.Errorf("expected message 'connection reset', got %q", err.Message)
	}

	if err.Duration != 500*time.Millisecond {
		t.Errorf("expected duration 500ms, got %v", err.Duration)
	}
}

func TestNewRetryExecutor(t *testing.T) {
	pm := newMockPoolManager()
	mb := newRetryMockBroker()
	secret := []byte("test-secret")

	executor := NewRetryExecutor(nil, nil, pm, mb, secret)

	if executor == nil {
		t.Fatal("expected executor to be created")
	}

	if executor.poolManager != pm {
		t.Error("expected poolManager to be set")
	}

	if executor.broker != mb {
		t.Error("expected broker to be set")
	}

	if string(executor.hmacSecret) != string(secret) {
		t.Error("expected hmacSecret to be set")
	}

	if executor.baseBackoff != DefaultBaseBackoff {
		t.Errorf("expected baseBackoff %v, got %v", DefaultBaseBackoff, executor.baseBackoff)
	}

	if executor.maxBackoff != DefaultMaxBackoff {
		t.Errorf("expected maxBackoff %v, got %v", DefaultMaxBackoff, executor.maxBackoff)
	}

	if executor.backoffFactor != DefaultBackoffFactor {
		t.Errorf("expected backoffFactor %v, got %v", DefaultBackoffFactor, executor.backoffFactor)
	}
}

func TestWithRetryLogger(t *testing.T) {
	executor := &RetryExecutor{}
	logger := slog.Default()

	opt := WithRetryLogger(logger)
	opt(executor)

	if executor.logger != logger {
		t.Error("expected logger to be set")
	}
}

func TestWithBackoffConfig(t *testing.T) {
	executor := &RetryExecutor{}

	opt := WithBackoffConfig(50*time.Millisecond, 2*time.Second, 3.0)
	opt(executor)

	if executor.baseBackoff != 50*time.Millisecond {
		t.Errorf("expected baseBackoff 50ms, got %v", executor.baseBackoff)
	}

	if executor.maxBackoff != 2*time.Second {
		t.Errorf("expected maxBackoff 2s, got %v", executor.maxBackoff)
	}

	if executor.backoffFactor != 3.0 {
		t.Errorf("expected backoffFactor 3.0, got %v", executor.backoffFactor)
	}
}

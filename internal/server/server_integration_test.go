package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/kwilabs/straw-proxy-server/internal/service/filter"
	"github.com/kwilabs/straw-proxy-server/internal/service/orchestrator"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

// --- Mocks ---

type MockApiKeyRepo struct {
	mock.Mock
}

func (m *MockApiKeyRepo) GetByID(ctx context.Context, id string) (*domain.ApiKey, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}
func (m *MockApiKeyRepo) Create(ctx context.Context, key *domain.ApiKey) error { return nil }
func (m *MockApiKeyRepo) Update(ctx context.Context, key *domain.ApiKey) error { return nil }
func (m *MockApiKeyRepo) Delete(ctx context.Context, id string) error          { return nil }
func (m *MockApiKeyRepo) List(ctx context.Context, limit, offset int) ([]domain.ApiKey, int, error) {
	return nil, 0, nil
}
func (m *MockApiKeyRepo) Revoke(ctx context.Context, id string) error { return nil }

type MockKeyCache struct {
	mock.Mock
}

func (m *MockKeyCache) GetKey(ctx context.Context, hash string) (*domain.ApiKey, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ApiKey), args.Error(1)
}
func (m *MockKeyCache) SetKey(ctx context.Context, hash string, key *domain.ApiKey) error {
	return m.Called(ctx, hash, key).Error(0)
}
func (m *MockKeyCache) InvalidateKey(ctx context.Context, hash string) error {
	return m.Called(ctx, hash).Error(0)
}
func (m *MockKeyCache) InvalidateKeyByID(ctx context.Context, id string) error {
	return nil
}

type MockRuleRepo struct {
	mock.Mock
}

func (m *MockRuleRepo) GetActiveRules(ctx context.Context) ([]domain.RoutingRule, error) {
	args := m.Called(ctx)
	return args.Get(0).([]domain.RoutingRule), args.Error(1)
}
func (m *MockRuleRepo) CreateRule(ctx context.Context, rule *domain.RoutingRule) error { return nil }
func (m *MockRuleRepo) GetRuleByID(ctx context.Context, id string) (*domain.RoutingRule, error) {
	return nil, nil
}
func (m *MockRuleRepo) UpdateRule(ctx context.Context, rule *domain.RoutingRule) error { return nil }
func (m *MockRuleRepo) DeleteRule(ctx context.Context, id string) error                { return nil }
func (m *MockRuleRepo) ListRules(ctx context.Context, limit, offset int) ([]domain.RoutingRule, int, error) {
	return nil, 0, nil
}

type MockRuleCache struct {
	mock.Mock
}

func (m *MockRuleCache) GetRulesVersion(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}
func (m *MockRuleCache) GetRulesByVersion(ctx context.Context, version int64) ([]domain.RoutingRule, error) {
	args := m.Called(ctx, version)
	return args.Get(0).([]domain.RoutingRule), args.Error(1)
}
func (m *MockRuleCache) SetRulesByVersion(ctx context.Context, version int64, rules []domain.RoutingRule) error {
	return m.Called(ctx, version, rules).Error(0)
}

type MockEndpointSelector struct {
	mock.Mock
}

func (m *MockEndpointSelector) Select(ctx context.Context, rule *domain.RoutingRule) (string, error) {
	args := m.Called(ctx, rule)
	return args.String(0), args.Error(1)
}
func (m *MockEndpointSelector) SelectWithSession(ctx context.Context, sessionID string) (string, error) {
	args := m.Called(ctx, sessionID)
	return args.String(0), args.Error(1)
}

type MockPoolManager struct {
	mock.Mock
}

func (m *MockPoolManager) GetEndpointFromPool(ctx context.Context, rule *domain.RoutingRule, poolTier int, exclude []string) (string, error) {
	args := m.Called(ctx, rule, poolTier, exclude)
	return args.String(0), args.Error(1)
}
func (m *MockPoolManager) GetPoolConfig(rule *domain.RoutingRule, poolTier int) *domain.EndpointPool {
	return nil
}

type MockBroker struct {
	mock.Mock
}

func (m *MockBroker) Publish(ctx context.Context, exchange, queue string, body []byte) error {
	args := m.Called(ctx, exchange, queue, body)
	return args.Error(0)
}
func (m *MockBroker) Consume(ctx context.Context, queue string, handler broker.Handler) error {
	args := m.Called(ctx, queue, handler)
	return args.Error(0)
}
func (m *MockBroker) Subscribe(ctx context.Context, queue string, handler broker.Handler) error {
	args := m.Called(ctx, queue, handler)
	return args.Error(0)
}
func (m *MockBroker) SubscribeTemporary(ctx context.Context, queue string, handler broker.Handler) error {
	args := m.Called(ctx, queue, handler)
	return args.Error(0)
}
func (m *MockBroker) ConsumeOnce(ctx context.Context, queue string, timeout time.Duration) ([]byte, error) {
	args := m.Called(ctx, queue, timeout)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}
func (m *MockBroker) Close() error                                                     { return nil }
func (m *MockBroker) DeclareExchange(ctx context.Context, name, kind string) error     { return nil }
func (m *MockBroker) BindQueue(ctx context.Context, queue, exchange, key string) error { return nil }
func (m *MockBroker) DeclareQueue(ctx context.Context, name string) error              { return nil }

func (m *MockBroker) IsConnected() bool {
	return true
}

func (m *MockBroker) QueueDepth(ctx context.Context, name string) (int, error) {
	return 0, nil
}

// --- Helper to hash password ---
func hashPassword(t *testing.T, password string) string {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}

// --- Tests ---

func TestServer_RelayRequest_Success(t *testing.T) {
	t.Skip("Skipping: This test requires significant refactoring to work with the shared queue pattern. Integration tests in test/integration/ cover this functionality.")
	// 1. Setup Mocks
	mockKeyRepo := new(MockApiKeyRepo)
	mockKeyCache := new(MockKeyCache)
	mockRuleRepo := new(MockRuleRepo)
	mockRuleCache := new(MockRuleCache)
	mockBroker := new(MockBroker)
	mockPoolManager := new(MockPoolManager)
	mockSelector := new(MockEndpointSelector)

	// 2. Setup Data
	apiKeyID := "test-key"
	apiSecret := "test-secret"
	apiKeyHash := hashPassword(t, apiSecret)

	apiKey := &domain.ApiKey{
		ID:       apiKeyID,
		KeyHash:  apiKeyHash,
		IsActive: true,
	}

	rawKey := apiKeyID + ":" + apiSecret

	// Prepare mocks for Auth
	mockKeyCache.On("GetKey", mock.Anything, mock.Anything).Return((*domain.ApiKey)(nil), nil)
	mockKeyRepo.On("GetByID", mock.Anything, apiKeyID).Return(apiKey, nil)
	mockKeyCache.On("SetKey", mock.Anything, mock.Anything, apiKey).Return(nil)

	// Prepare mocks for Matcher
	mockRuleCache.On("GetRulesVersion", mock.Anything).Return(int64(0), nil)

	rule := domain.RoutingRule{
		ID:           "rule-1",
		Priority:     100,
		RequiredTags: []string{},
		RequestFilters: &domain.RequestFilter{
			BlockDomains: []string{"blocked.com"},
		},
		EndpointPools: []domain.EndpointPool{{Tier: 1, Endpoints: []string{"start"}}},
	}
	mockRuleRepo.On("GetActiveRules", mock.Anything).Return([]domain.RoutingRule{rule}, nil)

	// Prepare mocks for Execution
	mockPoolManager.On("GetEndpointFromPool", mock.Anything, mock.Anything, 1, mock.Anything).Return("ep-1", nil)
	mockPoolManager.On("GetPoolConfig", mock.Anything, 1).Return(&rule.EndpointPools[0])

	// Mock Selector
	mockSelector.On("Select", mock.Anything, mock.Anything).Return("ep-1", nil)

	// Track the result queue handler for shared queue pattern
	var sharedQueueHandler broker.Handler

	// Mock Broker Subscribe for SharedResultQueue - capture the handler
	mockBroker.On("Subscribe", mock.Anything, orchestrator.SharedResultQueue, mock.MatchedBy(func(h broker.Handler) bool {
		sharedQueueHandler = h
		return true
	})).Return(nil)

	// Mock Broker Publish - capture request ID from the published task to send response
	var capturedRequestID string
	mockBroker.On("Publish", mock.Anything, mock.Anything, mock.AnythingOfType("string"), mock.Anything).Run(func(args mock.Arguments) {
		// Parse the published task to extract request ID
		body := args.Get(3).([]byte)
		var signedTask protocol.SignedTask
		if err := json.Unmarshal(body, &signedTask); err != nil {
			t.Logf("DEBUG: Failed to unmarshal signed task: %v", err)
			return
		}
		// Use ValidateSignedTask to decompress and parse the request
		req, err := protocol.ValidateSignedTask(&signedTask, []byte("secret"), time.Minute)
		if err != nil {
			t.Logf("DEBUG: Failed to validate signed task: %v", err)
			return
		}
		capturedRequestID = req.ID
		t.Logf("DEBUG: Captured request ID: %s", capturedRequestID)
	}).Return(nil)

	// Mock SubscribeTemporary if still called (legacy)
	mockBroker.On("SubscribeTemporary", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Return(nil).Maybe()

	// 3. Construct Server
	authSvc := auth.NewAuthService(mockKeyRepo, mockKeyCache)
	matcher := router.NewMatcher(mockRuleRepo, mockRuleCache)
	matcher.LoadRules(context.Background())

	cb := circuitbreaker.New(circuitbreaker.Config{})
	pub := orchestrator.NewPublisher(mockBroker, mockSelector, []byte("secret"), cb)
	sub := orchestrator.NewConsumer(mockBroker)
	executor := orchestrator.NewRetryExecutor(pub, sub, mockPoolManager, mockBroker, []byte("secret"))

	// Start the executor's shared queue consumer
	ctx := context.Background()
	err := executor.Start(ctx)
	assert.NoError(t, err)

	server := New(
		config.ServerConfig{HTTPPort: 0, MaxBodySize: "10M"},
		authSvc,
		nil,
		matcher,
		nil,
		filter.NewService(nil),
		executor,
	)

	// 4. Perform Request
	reqBody := `{"url": "http://example.com", "method": "GET"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/request", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", rawKey)

	rec := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		server.echo.ServeHTTP(rec, req)
		done <- true
	}()

	// Wait for publish to happen and capture request ID with polling
	var requestIDCaptured bool
	for i := 0; i < 50; i++ { // Poll for up to 5 seconds
		time.Sleep(100 * time.Millisecond)
		if capturedRequestID != "" {
			requestIDCaptured = true
			break
		}
	}

	if sharedQueueHandler != nil && requestIDCaptured {
		// Simulate Response via shared queue handler
		result := orchestrator.ResultMessage{
			RequestID:  capturedRequestID,
			EndpointID: "ep-1",
			StatusCode: 200,
			Headers: protocol.HeaderMap{
				{Key: "Content-Type", Value: "text/plain"},
			},
			CompressedBody: []byte("Restore the republic"),
			BodyCompressed: false,
		}
		bodyBytes, _ := json.Marshal(result)
		sharedQueueHandler(context.Background(), bodyBytes)
	} else {
		t.Logf("Warning: sharedQueueHandler=%v, requestIDCaptured=%v, capturedRequestID=%s",
			sharedQueueHandler != nil, requestIDCaptured, capturedRequestID)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("test timed out")
	}

	assert.Equal(t, http.StatusOK, rec.Code)
	// Check headers
	assert.Equal(t, "text/plain", rec.Header().Get("Content-Type"))
}

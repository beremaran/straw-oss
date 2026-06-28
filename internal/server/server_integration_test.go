package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/beremaran/straw/internal/service/auth"
	"github.com/beremaran/straw/internal/service/filter"
	"github.com/beremaran/straw/internal/service/orchestrator"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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
func (m *MockApiKeyRepo) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ApiKey, error) {
	args := m.Called(ctx, tokenHash)
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
func (m *MockApiKeyRepo) Exists(ctx context.Context) (bool, error)    { return false, nil }
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

func (m *MockBroker) Publish(ctx context.Context, subject string, body []byte) error {
	args := m.Called(ctx, subject, body)

	return args.Error(0)
}
func (m *MockBroker) Consume(ctx context.Context, queue string, handler broker.Handler) error {
	args := m.Called(ctx, queue, handler)

	return args.Error(0)
}
func (m *MockBroker) Subscribe(ctx context.Context, subject string, handler broker.Handler, opts ...broker.SubscribeOption) error {
	args := m.Called(ctx, subject, handler, opts)

	return args.Error(0)
}
func (m *MockBroker) ConsumeOnce(ctx context.Context, subject string, timeout time.Duration) ([]byte, error) {
	args := m.Called(ctx, subject, timeout)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	return args.Get(0).([]byte), args.Error(1)
}
func (m *MockBroker) Close() error { return nil }
func (m *MockBroker) DeclareStream(ctx context.Context, name string, subjects ...string) error {
	return nil
}
func (m *MockBroker) IsConnected() bool { return true }

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))

	return hex.EncodeToString(hash[:])
}

func TestServer_RelayRequest_Success(t *testing.T) {
	t.Skip("Skipping: This test requires significant refactoring to work with the shared subject pattern. Integration tests in test/integration/ cover this functionality.")

	mockKeyRepo := new(MockApiKeyRepo)
	mockKeyCache := new(MockKeyCache)
	mockRuleRepo := new(MockRuleRepo)
	mockRuleCache := new(MockRuleCache)
	mockBroker := new(MockBroker)
	mockPoolManager := new(MockPoolManager)
	mockSelector := new(MockEndpointSelector)

	bearerToken := "test-bearer-token-12345"
	tokenHash := sha256Hash(bearerToken)

	apiKey := &domain.ApiKey{
		ID:        "test-key-id",
		TokenHash: tokenHash,
		IsActive:  true,
	}

	mockKeyCache.On("GetKey", mock.Anything, tokenHash).Return((*domain.ApiKey)(nil), nil)
	mockKeyRepo.On("GetByTokenHash", mock.Anything, tokenHash).Return(apiKey, nil)
	mockKeyCache.On("SetKey", mock.Anything, tokenHash, apiKey).Return(nil)

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

	mockPoolManager.On("GetEndpointFromPool", mock.Anything, mock.Anything, 1, mock.Anything).Return("ep-1", nil)
	mockPoolManager.On("GetPoolConfig", mock.Anything, 1).Return(&rule.EndpointPools[0])

	mockSelector.On("Select", mock.Anything, mock.Anything).Return("ep-1", nil)

	var sharedQueueHandler broker.Handler

	mockBroker.On("Subscribe", mock.Anything, orchestrator.SharedResultSubject, mock.MatchedBy(func(h broker.Handler) bool {
		sharedQueueHandler = h

		return true
	}), mock.Anything).Return(nil)

	var capturedRequestID string
	mockBroker.On("Publish", mock.Anything, mock.AnythingOfType("string"), mock.Anything).Run(func(args mock.Arguments) {
		body := args.Get(2).([]byte)
		var signedTask protocol.SignedTask
		err := json.Unmarshal(body, &signedTask)
		if err != nil {
			t.Logf("DEBUG: Failed to unmarshal signed task: %v", err)

			return
		}

		req, err := protocol.ValidateSignedTask(&signedTask, []byte("secret"), time.Minute)
		if err != nil {
			t.Logf("DEBUG: Failed to validate signed task: %v", err)

			return
		}
		capturedRequestID = req.ID
		t.Logf("DEBUG: Captured request ID: %s", capturedRequestID)
	}).Return(nil)

	authSvc := auth.NewAuthService(mockKeyRepo, nil)
	matcher := router.NewMatcher(mockRuleRepo, nil)
	assert.NoError(t, matcher.LoadRules(context.Background()))

	cb := circuitbreaker.New(circuitbreaker.Config{})
	pub := orchestrator.NewPublisher(mockBroker, mockSelector, []byte("secret"), cb)
	sub := orchestrator.NewConsumer(mockBroker)
	executor := orchestrator.NewRetryExecutor(pub, sub, mockPoolManager, mockBroker, []byte("secret"))

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

	reqBody := `{"url": "http://example.com", "method": "GET"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/request", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearerToken)

	rec := httptest.NewRecorder()

	done := make(chan bool)
	go func() {
		server.server.Handler.ServeHTTP(rec, req)
		done <- true
	}()

	var requestIDCaptured bool
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if capturedRequestID != "" {
			requestIDCaptured = true

			break
		}
	}

	if sharedQueueHandler != nil && requestIDCaptured {
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
		bodyBytes, err := json.Marshal(result)
		assert.NoError(t, err)
		assert.NoError(t, sharedQueueHandler(context.Background(), bodyBytes))
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

	assert.Equal(t, "text/plain", rec.Header().Get("Content-Type"))
}

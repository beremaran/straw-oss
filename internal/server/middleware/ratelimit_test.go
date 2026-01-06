package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/server/middleware"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int {
	return &i
}

// MockRuleRepo mocks router.RuleRepository
type MockRuleRepo struct {
	mock.Mock
}

func (m *MockRuleRepo) GetActiveRules(ctx context.Context) ([]domain.RoutingRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)

	}
	return args.Get(0).([]domain.RoutingRule), args.Error(1)
}

// MockRuleCache mocks router.RuleCacheInterface
type MockRuleCache struct {
	mock.Mock
}

func (m *MockRuleCache) GetRulesVersion(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockRuleCache) GetRulesByVersion(ctx context.Context, version int64) ([]domain.RoutingRule, error) {
	args := m.Called(ctx, version)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.RoutingRule), args.Error(1)
}

func (m *MockRuleCache) SetRulesByVersion(ctx context.Context, version int64, rules []domain.RoutingRule) error {
	args := m.Called(ctx, version, rules)
	return args.Error(0)
}

func TestRateLimitMiddleware(t *testing.T) {
	// 1. Setup Redis & Limiter
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	client, err := redis.NewClient(config.CoreConfig{RedisAddr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)

	// 2. Setup Matcher with Rules
	mockRepo := new(MockRuleRepo)
	mockCache := new(MockRuleCache)
	// Simulate cache miss (no version)
	mockCache.On("GetRulesVersion", mock.Anything).Return(int64(0), nil)
	// mockCache.On("GetRulesByVersion", ... ) // Not called if version is 0
	mockCache.On("SetRulesByVersion", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	rules := []domain.RoutingRule{
		{
			ID:                 "rule-amazon",
			Name:               "Amazon Rule",
			RequiredTags:       []string{"target:amazon"},
			RateLimitPerMinute: 2,
			QuotaKey:           "target:amazon",
			IsActive:           true,
			Priority:           10,
		},
		{
			ID:           "rule-general",
			Name:         "General Rule",
			RequiredTags: []string{"type:general"},
			// No limits
			IsActive: true,
			Priority: 5,
		},
	}
	mockRepo.On("GetActiveRules", mock.Anything).Return(rules, nil)

	matcher := router.NewMatcher(mockRepo, mockCache)
	err = matcher.LoadRules(context.Background())
	require.NoError(t, err)

	// 3. Setup Echo
	e := echo.New()
	mw := middleware.RateLimitMiddleware(limiter, matcher)

	// Handler that asserts rule is present
	handler := func(c echo.Context) error {
		rule := middleware.GetRoutingRule(c)
		if rule == nil {
			return c.String(http.StatusInternalServerError, "no rule in context")
		}
		return c.String(http.StatusOK, "success:"+rule.ID)
	}

	t.Run("Allows request under limit", func(t *testing.T) {
		s.FlushAll()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		// mock X-Relay-Tags
		req.Header.Set("X-Relay-Tags", "target=amazon")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Set dummy API Key
		c.Set("api_key", &domain.ApiKey{ID: "test-user"})

		err := mw(handler)(c)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "success:rule-amazon")
		assert.Equal(t, "2", rec.Header().Get("X-RateLimit-Limit"))
		assert.Equal(t, "1", rec.Header().Get("X-RateLimit-Remaining"))
	})

	t.Run("Blocks request over limit", func(t *testing.T) {
		s.FlushAll()
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Relay-Tags", "target=amazon")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.Set("api_key", &domain.ApiKey{ID: "test-user"})

			err := mw(handler)(c)
			if i < 2 {
				assert.NoError(t, err)
				assert.Equal(t, http.StatusOK, rec.Code)
			} else {
				// Third request should be blocked (429)
				assert.NoError(t, err)
				assert.Equal(t, http.StatusTooManyRequests, rec.Code)
				assert.Contains(t, rec.Body.String(), "RATE_LIMIT_EXCEEDED")
				assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))
			}
		}
	})

	t.Run("Fails if no rule matches", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Relay-Tags", "target=uknown")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		err := mw(handler)(c)
		// Should return 503 HTTPError
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		assert.True(t, ok)
		assert.Equal(t, http.StatusServiceUnavailable, httpErr.Code)
	})

	t.Run("Uses API Key override", func(t *testing.T) {
		s.FlushAll()
		// Rule has limit 2. Key has override 10.
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Relay-Tags", "target=amazon")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		c.Set("api_key", &domain.ApiKey{ID: "whale-user", RateLimitOverride: intPtr(10)})

		err := mw(handler)(c)
		assert.NoError(t, err)
		assert.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
	})
}

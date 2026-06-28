package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/middleware"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int {
	return &i
}

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

func (m *MockRuleRepo) CreateRule(ctx context.Context, rule *domain.RoutingRule) error { return nil }
func (m *MockRuleRepo) GetRuleByID(ctx context.Context, id string) (*domain.RoutingRule, error) {
	return nil, nil
}
func (m *MockRuleRepo) UpdateRule(ctx context.Context, rule *domain.RoutingRule) error { return nil }
func (m *MockRuleRepo) DeleteRule(ctx context.Context, id string) error                { return nil }
func (m *MockRuleRepo) ListRules(ctx context.Context, limit, offset int) ([]domain.RoutingRule, int, error) {
	return nil, 0, nil
}

func TestRateLimitMiddleware(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	ctx := t.Context()
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: s.Addr()}, nil)
	require.NoError(t, err)
	defer client.Close()

	limiter := ratelimit.NewRateLimiter(client)

	mockRepo := new(MockRuleRepo)
	ruleCache := router.NewRuleCache(client.Client, time.Minute)

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
			IsActive:     true,
			Priority:     5,
		},
	}
	mockRepo.On("GetActiveRules", mock.Anything).Return(rules, nil)

	matcher := router.NewMatcher(mockRepo, ruleCache)
	err = matcher.LoadRules(context.Background())
	require.NoError(t, err)

	mw := middleware.RateLimitMiddleware(limiter, matcher)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rule := middleware.GetRoutingRule(r)
		if rule == nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("no rule in context"))

			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success:" + rule.ID))
	})

	t.Run("Allows request under limit", func(t *testing.T) {
		s.FlushAll()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("X-Relay-Tags", "target=amazon")
		rec := httptest.NewRecorder()

		ctx := context.WithValue(req.Context(), middleware.ContextApiKey{Value: "api_key"}, &domain.ApiKey{ID: "test-user"})
		req = req.WithContext(ctx)

		mw(handler).ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "success:rule-amazon")
		assert.Equal(t, "2", rec.Header().Get("X-RateLimit-Limit"))
		assert.Equal(t, "1", rec.Header().Get("X-RateLimit-Remaining"))
	})

	t.Run("Blocks request over limit", func(t *testing.T) {
		s.FlushAll()
		for i := 0; i < 3; i++ {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			req.Header.Set("X-Relay-Tags", "target=amazon")
			rec := httptest.NewRecorder()

			ctx := context.WithValue(req.Context(), middleware.ContextApiKey{Value: "api_key"}, &domain.ApiKey{ID: "test-user"})
			req = req.WithContext(ctx)

			mw(handler).ServeHTTP(rec, req)
			if i < 2 {
				assert.Equal(t, http.StatusOK, rec.Code)
			} else {
				assert.Equal(t, http.StatusTooManyRequests, rec.Code)
				assert.Contains(t, rec.Body.String(), "RATE_LIMIT_EXCEEDED")
				assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))
			}
		}
	})

	t.Run("Fails if no rule matches", func(t *testing.T) {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("X-Relay-Tags", "target=uknown")
		rec := httptest.NewRecorder()

		mw(handler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("Uses API Key override", func(t *testing.T) {
		s.FlushAll()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
		req.Header.Set("X-Relay-Tags", "target=amazon")
		rec := httptest.NewRecorder()

		ctx := context.WithValue(req.Context(), middleware.ContextApiKey{Value: "api_key"}, &domain.ApiKey{ID: "whale-user", RateLimitOverride: intPtr(10)})
		req = req.WithContext(ctx)

		mw(handler).ServeHTTP(rec, req)
		assert.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
	})
}

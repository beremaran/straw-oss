package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/kwilabs/straw-proxy-server/internal/server"
	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/kwilabs/straw-proxy-server/internal/service/filter"
	"github.com/kwilabs/straw-proxy-server/internal/service/orchestrator"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/kwilabs/straw-proxy-server/test/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestValidation_SecurityScenarios(t *testing.T) {
	// Setup shared suite (reuses containers)
	s := integration.GetSuite(t)
	s.CleanupForTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Setup Server Components
	serverConf := integration.NewTestServerConfig(s.PostgresDSN(), s.RedisAddr(), s.RabbitMQURL())

	authRepo := integration.NewTestAuthRepo(t, s.PostgresDSN())
	rlRedis := integration.NewTestRedisClient(t, s.RedisAddr())
	authCache := auth.NewAuthCache(rlRedis, 10*time.Second)
	authService := auth.NewAuthService(authRepo, authCache)
	sessionRepo := session.NewRedisStore(rlRedis)
	sessionService := session.NewService(sessionRepo)
	ruleRepo := integration.NewTestRuleRepo(t, s.PostgresDSN())
	ruleCache := router.NewRuleCache(rlRedis.Client, 1*time.Minute)
	matcher := router.NewMatcher(ruleRepo, ruleCache)
	err := matcher.LoadRules(ctx)
	require.NoError(t, err)
	rateLimiter := ratelimit.NewRateLimiter(rlRedis)
	filterService := filter.NewService(nil)
	rabbitmq := broker.NewRabbitMQBroker(broker.Addrs(s.RabbitMQURL()))
	err = rabbitmq.Connect()
	require.NoError(t, err)
	defer rabbitmq.Close()
	cb := circuitbreaker.New(circuitbreaker.Config{})
	selector := &dummySelector{}
	pub := orchestrator.NewPublisher(rabbitmq, selector, []byte("secret"), cb)
	sub := orchestrator.NewConsumer(rabbitmq)
	poolMgr := &dummyPoolManager{}
	executor := orchestrator.NewRetryExecutor(pub, sub, poolMgr, rabbitmq, []byte("secret"))

	srv := server.New(*serverConf, authService, sessionService, matcher, rateLimiter, filterService, executor)

	// Create Valid API Key for testing
	key, err := integration.CreateTestAPIKey(ctx, s.PostgresDSN(), "ValidationKey", []string{"*"})
	require.NoError(t, err)

	// Helper
	sendRequest := func(method, url string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("X-API-Key", key.RawKey)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.GetEcho().ServeHTTP(rec, req)
		return rec
	}

	t.Run("OversizedRequest_Rejection", func(t *testing.T) {
		// MaxBodySize is "2M" (2MB) set in helpers.go NewTestServerConfig
		// Create a body > 2MB
		largeBody := `{"url": "http://example.com", "body": "` + strings.Repeat("a", 2*1024*1024+10) + `"}`

		rec := sendRequest("POST", "/v1/request", largeBody)

		// Echo returns 413 Request Entity Too Large
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})

	t.Run("InternalIP_Blocking_SSRF", func(t *testing.T) {
		// Test various internal/local addresses
		targets := []string{
			"http://localhost:8080",
			"http://127.0.0.1:22",
			"http://0.0.0.0:80",
			"http://192.168.1.1",
			"http://10.0.0.5",
			"http://[::1]",
		}

		for _, target := range targets {
			t.Run("Target_"+target, func(t *testing.T) {
				// Must construct valid JSON request
				body := `{"url": "` + target + `", "method": "GET"}`
				rec := sendRequest("POST", "/v1/request", body)

				// Should be rejected by validator
				// handlers/relay.go -> validator.ValidateTargetURL -> returns 403 Forbidden
				assert.Equal(t, http.StatusForbidden, rec.Code, "Expected target %s to be blocked", target)
				assert.Contains(t, rec.Body.String(), "invalid target url")
			})
		}
	})
}

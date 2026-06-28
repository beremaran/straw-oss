package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/beremaran/straw/internal/server"
	"github.com/beremaran/straw/internal/service/auth"
	"github.com/beremaran/straw/internal/service/filter"
	"github.com/beremaran/straw/internal/service/orchestrator"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/internal/service/session"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/test/integration"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestValidation_SecurityScenarios(t *testing.T) {

	s := integration.GetSuite(t)
	s.CleanupForTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	serverConf := integration.NewTestServerConfig(s.PostgresDSN(), s.RedisAddr(), s.NatsURL())

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
	natsBroker := broker.NewNatsBroker(broker.Addrs(s.NatsURL()))
	err = natsBroker.Connect()
	require.NoError(t, err)
	defer natsBroker.Close()
	cb := circuitbreaker.New(circuitbreaker.Config{})
	selector := &dummySelector{}
	pub := orchestrator.NewPublisher(natsBroker, selector, []byte("secret"), cb)
	sub := orchestrator.NewConsumer(natsBroker)
	poolMgr := &dummyPoolManager{}
	executor := orchestrator.NewRetryExecutor(pub, sub, poolMgr, natsBroker, []byte("secret"))

	srv := server.New(*serverConf, authService, sessionService, matcher, rateLimiter, filterService, executor)

	key, err := integration.CreateTestAPIKey(ctx, s.PostgresDSN(), "ValidationKey", []string{"*"})
	require.NoError(t, err)

	sendRequest := func(method, url string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+key.RawKey)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.GetHandler().ServeHTTP(rec, req)
		return rec
	}

	t.Run("OversizedRequest_Rejection", func(t *testing.T) {

		largeBody := `{"url": "http://example.com", "body": "` + strings.Repeat("a", 2*1024*1024+10) + `"}`

		rec := sendRequest("POST", "/v1/request", largeBody)

		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	})

	t.Run("InternalIP_Blocking_SSRF", func(t *testing.T) {

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

				body := `{"url": "` + target + `", "method": "GET"}`
				rec := sendRequest("POST", "/v1/request", body)

				assert.Equal(t, http.StatusForbidden, rec.Code, "Expected target %s to be blocked", target)
				assert.Contains(t, rec.Body.String(), "invalid target url")
			})
		}
	})
}

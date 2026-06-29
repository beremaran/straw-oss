package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
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

func TestAuthentication_SecurityScenarios(t *testing.T) {
	s := integration.GetSuite(t)
	s.CleanupForTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	serverConf := integration.NewTestServerConfig(s.PostgresDSN(), s.RedisAddr(), s.NatsURL())

	authRepo := integration.NewTestAuthRepo(t, s.PostgresDSN())
	authTokenRepo := integration.NewTestAuthTokenRepo(t, s.PostgresDSN())

	rlRedis := integration.NewTestRedisClient(t, ctx, s.RedisAddr())
	authCache := auth.NewAuthCache(rlRedis, 10*time.Second)
	authService := auth.NewAuthService(authRepo, authTokenRepo, authCache)

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
	defer func() { _ = natsBroker.Close() }()

	cb := circuitbreaker.New(circuitbreaker.Config{})
	selector := &dummySelector{}

	pub := orchestrator.NewPublisher(natsBroker, selector, []byte("test-hmac-secret-for-integration-tests"), cb)
	sub := orchestrator.NewConsumer(natsBroker)

	poolMgr := &dummyPoolManager{}

	executor := orchestrator.NewRetryExecutor(pub, sub, poolMgr, natsBroker, []byte("test-hmac-secret-for-integration-tests"))

	srv := server.New(*serverConf, authService, sessionService, matcher, rateLimiter, filterService, executor)

	sendRequest := func(method, url, apiToken string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(context.Background(), method, url, nil)
		if apiToken != "" {
			req.Header.Set("Authorization", "Bearer "+apiToken)
		}
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.GetHandler().ServeHTTP(rec, req)

		return rec
	}

	t.Run("BruteForceProtection_InvalidKeys", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			rec := sendRequest("POST", "/v1/request", "invalid-key")
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		}
	})

	t.Run("RevokedKey_ImmediateRejection", func(t *testing.T) {
		key, err := integration.CreateTestAPIKey(ctx, s.PostgresDSN(), "To Be Revoked", []string{"*"})
		require.NoError(t, err)

		rec := sendRequest("POST", "/v1/request", key.RawKey)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "Key should be valid initially")

		integration.ExecuteSQL(t, ctx, s.PostgresDSN(), "UPDATE api_keys SET is_active = false WHERE id = $1", key.ID)

		err = authService.InvalidateKey(ctx, key.RawKey)
		require.NoError(t, err)

		rec = sendRequest("POST", "/v1/request", key.RawKey)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Revoked key should be rejected")
	})

	t.Run("KeyRotation_GracePeriod", func(t *testing.T) {
		key, err := integration.CreateTestAPIKey(ctx, s.PostgresDSN(), "Rotation Test", []string{"*"})
		require.NoError(t, err)

		rec := sendRequest("POST", "/v1/request", key.RawKey)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code)

		newToken := "new-rotated-token-value"
		newHashBytes := sha256.Sum256([]byte(newToken))
		newHash := hex.EncodeToString(newHashBytes[:])

		integration.ExecuteSQL(t, ctx, s.PostgresDSN(), "INSERT INTO api_key_tokens (api_key_id, token_hash, status) VALUES ($1, $2, 'active')", key.ID, newHash)
		integration.ExecuteSQL(t, ctx, s.PostgresDSN(), "UPDATE api_key_tokens SET status = 'grace', expires_at = NOW() + INTERVAL '1 hour' WHERE api_key_id = $1 AND token_hash != $2", key.ID, newHash)

		require.NoError(t, authService.InvalidateKey(ctx, key.RawKey))

		rec = sendRequest("POST", "/v1/request", key.RawKey)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "Old token should work during grace period")

		integration.ExecuteSQL(t, ctx, s.PostgresDSN(), "UPDATE api_key_tokens SET expires_at = NOW() - INTERVAL '1 hour' WHERE api_key_id = $1 AND token_hash != $2", key.ID, newHash)
		require.NoError(t, authService.InvalidateKey(ctx, key.RawKey))

		rec = sendRequest("POST", "/v1/request", key.RawKey)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Old token should fail after grace period expires")

		rec = sendRequest("POST", "/v1/request", newToken)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "New token should work")
	})
}

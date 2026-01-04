package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
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

// TestAuthentication_SecurityScenarios covers security-specific auth tests.
func TestAuthentication_SecurityScenarios(t *testing.T) {
	// Setup shared suite
	s := integration.GetSuite(t)
	s.CleanupForTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Setup Server Components
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

	pub := orchestrator.NewPublisher(natsBroker, selector, []byte("test-hmac-secret-for-integration-tests"), cb)
	sub := orchestrator.NewConsumer(natsBroker)

	poolMgr := &dummyPoolManager{}

	executor := orchestrator.NewRetryExecutor(pub, sub, poolMgr, natsBroker, []byte("test-hmac-secret-for-integration-tests"))

	srv := server.New(*serverConf, authService, sessionService, matcher, rateLimiter, filterService, executor)

	// Helper
	sendRequest := func(method, url, apiToken string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, nil)
		if apiToken != "" {
			req.Header.Set("Authorization", "Bearer "+apiToken)
		}
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.GetEcho().ServeHTTP(rec, req)
		return rec
	}

	t.Run("BruteForceProtection_InvalidKeys", func(t *testing.T) {
		for i := 0; i < 20; i++ {
			rec := sendRequest("POST", "/v1/request", "invalid-key")
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		}
	})

	t.Run("RevokedKey_ImmediateRejection", func(t *testing.T) {
		// 1. Create a valid key
		key, err := integration.CreateTestAPIKey(ctx, s.PostgresDSN(), "To Be Revoked", []string{"*"})
		require.NoError(t, err)

		// 2. Verify it generally works
		rec := sendRequest("POST", "/v1/request", key.RawKey)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "Key should be valid initially")

		// 3. Revoke Key
		integration.ExecuteSQL(t, s.PostgresDSN(), "UPDATE api_keys SET is_active = false WHERE id = $1", key.ID)

		// 4. Invalidate Cache
		err = authService.InvalidateKey(ctx, key.RawKey)
		require.NoError(t, err)

		// 5. Verify rejection
		rec = sendRequest("POST", "/v1/request", key.RawKey)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Revoked key should be rejected")
	})

	t.Run("KeyRotation_GracePeriod", func(t *testing.T) {
		// 1. Create Key
		key, err := integration.CreateTestAPIKey(ctx, s.PostgresDSN(), "Rotation Test", []string{"*"})
		require.NoError(t, err)

		// Verify works
		rec := sendRequest("POST", "/v1/request", key.RawKey)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code)

		// 2. Rotate Key (Update Hash in DB with new token)
		newToken := "new-rotated-token-value"
		newHashBytes := sha256.Sum256([]byte(newToken))
		newHash := hex.EncodeToString(newHashBytes[:])

		integration.ExecuteSQL(t, s.PostgresDSN(), "UPDATE api_keys SET token_hash = $1 WHERE id = $2", newHash, key.ID)

		// 3. Invalidate cache
		authService.InvalidateKey(ctx, key.RawKey)

		// 4. Old Key should fail
		rec = sendRequest("POST", "/v1/request", key.RawKey)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "Old token should fail after rotation")

		// 5. New Token should work
		rec = sendRequest("POST", "/v1/request", newToken)
		assert.NotEqual(t, http.StatusUnauthorized, rec.Code, "New token should work")
	})
}

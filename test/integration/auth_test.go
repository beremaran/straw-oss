//nolint:errcheck
package integration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticationFlows(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	suite := GetSuite(t)
	ctx := context.Background()
	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	createRule := func(name string, priority int, reqTags []string, rateLimit int) {
		err := CreateTestRoutingRule(ctx, suite.PostgresDSN(), name, priority, reqTags, nil, name, rateLimit, 0, "", []TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 1},
		})
		require.NoError(t, err)
	}

	createRule("AuthTestRule", 100, []string{"target:auth_test"}, 100)

	err := CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"target:auth_test", "target:scope_test", "target:rate_limit_test"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	tc.ReplaceMockEndpoint(NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "test-endpoint-1",
		Secret:     []byte(testHMACSecret),
		TargetURL:  tc.MockTarget.URL(),
		Tags:       []string{"target:auth_test", "target:scope_test", "target:rate_limit_test"},
	}))
	require.NoError(t, tc.MockEndpoint.Start(ctx))
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	t.Run("Valid API Key", func(t *testing.T) {
		keyName := "valid-user"
		apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), keyName, []string{"*"})
		require.NoError(t, err)

		client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Invalid API Key", func(t *testing.T) {
		client := NewHTTPTestClient(tc.ServerURL, "invalid-key-format")
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		client = NewHTTPTestClient(tc.ServerURL, uuid.New().String()+":secret_123")
		resp, err = client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Expired API Key", func(t *testing.T) {
		keyID := uuid.New().String()
		token := "expired-token-" + uuid.New().String()
		tokenHashBytes := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(tokenHashBytes[:])

		db, err := sql.Open("pgx", suite.PostgresDSN())
		require.NoError(t, err)
		defer db.Close()

		_, err = db.ExecContext(ctx, `
			INSERT INTO api_keys (id, name, token_hash, scopes, is_active, created_at, expires_at)
			VALUES ($1, $2, $3, '[]', true, NOW() - INTERVAL '2 days', NOW() - INTERVAL '1 day')
		`, keyID, "expired-user", tokenHash)
		require.NoError(t, err)

		client := NewHTTPTestClient(tc.ServerURL, token)
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Scope Restriction", func(t *testing.T) {
		createRule("ScopeTestRule", 200, []string{"target:scope_test"}, 100)
		require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

		restrictedKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "restricted-user", []string{"target:auth_test"})
		require.NoError(t, err)

		client := NewHTTPTestClient(tc.ServerURL, restrictedKey.RawKey)

		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		resp, err = client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:scope_test"},
		})
		require.NoError(t, err)

		if resp.StatusCode == http.StatusOK {
			t.Log("FAILURE EXPECTED: Scope Restriction not implemented yet")
		}
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, resp.StatusCode, "Expected Forbidden or Unauthorized for scope violation")
	})

	t.Run("Rate Limit Override", func(t *testing.T) {
		createRule("RateLimitRule", 300, []string{"target:rate_limit_test"}, 1)
		require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

		stdKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "std-user", []string{"*"})
		require.NoError(t, err)

		clientStd := NewHTTPTestClient(tc.ServerURL, stdKey.RawKey)

		resp, err := clientStd.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:rate_limit_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		resp, err = clientStd.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:rate_limit_test"},
		})
		require.NoError(t, err)
		if resp.StatusCode != http.StatusTooManyRequests {
			resp, _ = clientStd.SendRequest(ctx, &ProxyRequest{
				URL:  tc.MockTarget.URL(),
				Tags: []string{"target:rate_limit_test"},
			})
		}
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Expected standard key to be rate limited")

		keyID := uuid.New().String()
		premiumToken := "premium-token-" + uuid.New().String()
		tokenHashBytes := sha256.Sum256([]byte(premiumToken))
		tokenHash := hex.EncodeToString(tokenHashBytes[:])

		db, err := sql.Open("pgx", suite.PostgresDSN())
		require.NoError(t, err)
		defer db.Close()

		_, err = db.ExecContext(ctx, `
			INSERT INTO api_keys (id, name, token_hash, scopes, rate_limit_override, is_active, created_at)
			VALUES ($1, $2, $3, '[]', 10, true, NOW())
		`, keyID, "premium-user", tokenHash)
		require.NoError(t, err)

		clientPrem := NewHTTPTestClient(tc.ServerURL, premiumToken)

		time.Sleep(1100 * time.Millisecond)

		successCount := 0
		for i := 0; i < 5; i++ {
			resp, err := clientPrem.SendRequest(ctx, &ProxyRequest{
				URL:  tc.MockTarget.URL(),
				Tags: []string{"target:rate_limit_test"},
			})
			require.NoError(t, err)
			if resp.StatusCode == http.StatusOK {
				successCount++
			}
		}

		if successCount < 5 {
			t.Logf("FAILURE EXPECTED: Rate Limit Override not implemented yet. Success count: %d", successCount)
		} else {
			assert.Equal(t, 5, successCount, "Premium key should bypass rule rate limit")
		}
		assert.Equal(t, 5, successCount, "Premium key should bypass rule rate limit")
	})
}

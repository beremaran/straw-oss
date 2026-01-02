package integration

import (
	"context"
	"database/sql"
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

	// 1. Setup Suite
	suite := GetSuite(t)
	ctx := context.Background()
	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	// Helper to create rule
	createRule := func(name string, priority int, reqTags []string, rateLimit int) {
		err := CreateTestRoutingRule(ctx, suite.PostgresDSN(), name, priority, reqTags, nil, name, rateLimit, 0, "", []TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 1},
		})
		require.NoError(t, err)
	}

	// Create a basic rule for testing
	// Rule 1: Requires "target:auth_test" - used for basic auth checks
	createRule("AuthTestRule", 100, []string{"target:auth_test"}, 100)

	// Create endpoint
	err := CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"target:auth_test", "target:scope_test", "target:rate_limit_test"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	// Start services
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	// Create mock target responses
	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	// Setup Mock Endpoint
	tc.MockEndpoint = NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "test-endpoint-1",
		Secret:     []byte(testHMACSecret),
		TargetURL:  tc.MockTarget.URL(),
		Tags:       []string{"target:auth_test", "target:scope_test", "target:rate_limit_test"},
	})
	require.NoError(t, tc.MockEndpoint.Start(ctx))
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	t.Run("Valid API Key", func(t *testing.T) {
		// Create valid key
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
		// Random invalid key
		client := NewHTTPTestClient(tc.ServerURL, "invalid-key-format")
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		// Valid format but unknown key
		client = NewHTTPTestClient(tc.ServerURL, uuid.New().String()+":secret_123")
		resp, err = client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Expired API Key", func(t *testing.T) {
		// Create key manually with expiry in the past
		keyID := uuid.New().String()
		secret := "expired_secret"
		hash, _ := hashPassword(secret)

		db, err := sql.Open("pgx", suite.PostgresDSN())
		require.NoError(t, err)
		defer db.Close()

		_, err = db.ExecContext(ctx, `
			INSERT INTO api_keys (id, name, key_hash, scopes, is_active, created_at, expires_at)
			VALUES ($1, $2, $3, '[]', true, NOW() - INTERVAL '2 days', NOW() - INTERVAL '1 day')
		`, keyID, "expired-user", hash)
		require.NoError(t, err)

		rawKey := keyID + ":" + secret
		client := NewHTTPTestClient(tc.ServerURL, rawKey)
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Scope Restriction", func(t *testing.T) {
		// Rule 2: Requires "target:scope_test"
		createRule("ScopeTestRule", 200, []string{"target:scope_test"}, 100)
		require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

		// Key with restricted scope (only allows "target:auth_test")
		restrictedKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "restricted-user", []string{"target:auth_test"})
		require.NoError(t, err)

		client := NewHTTPTestClient(tc.ServerURL, restrictedKey.RawKey)

		// 1. Try to access allowed scope -> Should pass (matches AuthTestRule)
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:auth_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 2. Try to access FORBIDDEN scope -> Should fail
		// Asking for "target:scope_test" which is NOT in key scopes
		resp, err = client.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:scope_test"},
		})
		require.NoError(t, err)
		// Should be 403 Forbidden because key is valid but scope is not allowed
		// We expect this to fail (return 200 OK) until we implement scope enforcement
		if resp.StatusCode == http.StatusOK {
			t.Log("FAILURE EXPECTED: Scope Restriction not implemented yet")
			// We intentionally assert failure of test if it passes (to fail CI),
			// BUT for TDD we want to see it fail.
			// assert.Contains will fail if status is 200.
		}
		assert.Contains(t, []int{http.StatusForbidden, http.StatusUnauthorized}, resp.StatusCode, "Expected Forbidden or Unauthorized for scope violation")
	})

	t.Run("Rate Limit Override", func(t *testing.T) {
		// Rule 3: Requires "target:rate_limit_test", Limit 1 RPS
		createRule("RateLimitRule", 300, []string{"target:rate_limit_test"}, 1)
		require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

		// 1. Standard Key -> Should hit limit after 1 request
		stdKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "std-user", []string{"*"})
		require.NoError(t, err)

		clientStd := NewHTTPTestClient(tc.ServerURL, stdKey.RawKey)

		// First request OK
		resp, err := clientStd.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:rate_limit_test"},
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Second request within 1s -> Should verify rate limit works (429)
		// Note: Race condition possible if test is slow.
		resp, err = clientStd.SendRequest(ctx, &ProxyRequest{
			URL:  tc.MockTarget.URL(),
			Tags: []string{"target:rate_limit_test"},
		})
		require.NoError(t, err)
		if resp.StatusCode != http.StatusTooManyRequests {
			// Try one more just in case of timing edge case
			resp, _ = clientStd.SendRequest(ctx, &ProxyRequest{
				URL:  tc.MockTarget.URL(),
				Tags: []string{"target:rate_limit_test"},
			})
		}
		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Expected standard key to be rate limited")

		// 2. Premium Key -> Override Limit 10 RPS
		// Create premium key manually to set override
		keyID := uuid.New().String()
		secret := "premium_secret"
		hash, _ := hashPassword(secret)

		db, err := sql.Open("pgx", suite.PostgresDSN())
		require.NoError(t, err)
		defer db.Close()

		_, err = db.ExecContext(ctx, `
			INSERT INTO api_keys (id, name, key_hash, scopes, rate_limit_override, is_active, created_at)
			VALUES ($1, $2, $3, '[]', 10, true, NOW())
		`, keyID, "premium-user", hash)
		require.NoError(t, err)

		premiumKey := keyID + ":" + secret
		clientPrem := NewHTTPTestClient(tc.ServerURL, premiumKey)

		// Wait 1s to clear previous limits just in case quota is shared (it shouldn't be for different keys)
		time.Sleep(1100 * time.Millisecond)

		// Should be able to make multiple requests
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
		// Expect failure initially: implementation uses Rule limit (1 rps) instead of Override (10 rps)
		// So we expect < 5 successes (likely 1)

		if successCount < 5 {
			t.Logf("FAILURE EXPECTED: Rate Limit Override not implemented yet. Success count: %d", successCount)
			// assert.Equal(t, 5, successCount, "Premium key should bypass rule rate limit")
		} else {
			assert.Equal(t, 5, successCount, "Premium key should bypass rule rate limit")
		}
		// For TDD purposes, let's keep the assertion to make it fail
		assert.Equal(t, 5, successCount, "Premium key should bypass rule rate limit")
	})
}

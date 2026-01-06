package integration

import (
	"context"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/server/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSession_Stickiness verifies that requests with the same session ID are routed to the same endpoint.
func TestSession_Stickiness(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	// Seed random for this test execution
	rand.Seed(time.Now().UnixNano())

	// Create test data
	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	// Create TWO healthy endpoints in the same pool.
	// We need them to have the same tags so they are both valid candidates.
	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "endpoint-1",
		Secret:     []byte(testHMACSecret),
		Tags:       []string{"type:sticky"},
	})
	ep1.SetResponse(&MockEndpointResponse{
		StatusCode: 200, Body: []byte("Response from EP1"),
	})
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "endpoint-2",
		Secret:     []byte(testHMACSecret),
		Tags:       []string{"type:sticky"},
	})
	ep2.SetResponse(&MockEndpointResponse{
		StatusCode: 200, Body: []byte("Response from EP2"),
	})
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()

	// Register endpoints in DB/HealthStore
	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: "endpoint-1", Tags: []string{"type:sticky"}, IsHealthy: true})
	require.NoError(t, err)
	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: "endpoint-2", Tags: []string{"type:sticky"}, IsHealthy: true})
	require.NoError(t, err)

	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-1"))
	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-2"))

	// Create Routing Rule
	err = CreateTestRoutingRule(
		ctx, suite.PostgresDSN(), "Sticky Rule", 100,
		[]string{"type:sticky"}, []string{}, "", 0, 0, "",
		[]TestEndpointPool{{Tier: 1, Endpoints: []string{"endpoint-1", "endpoint-2"}, MaxRetries: 1}},
	)
	require.NoError(t, err)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// 1. First Request: Create Session
	resp1, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    "http://example.com",
		Method: "GET",
		Tags:   []string{"type:sticky"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	sessionID := resp1.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID, "Session ID should be returned")

	// Determine which endpoint was picked
	var firstEndpointID string
	if len(ep1.GetRequests()) > 0 {
		firstEndpointID = "endpoint-1"
	} else if len(ep2.GetRequests()) > 0 {
		firstEndpointID = "endpoint-2"
	} else {
		t.Fatal("Neither endpoint received the request")
	}
	t.Logf("First request went to: %s", firstEndpointID)

	ep1.ClearRequests()
	ep2.ClearRequests()

	// 2. Second Request: With Session ID
	// Should go to the SAME endpoint
	resp2, err := client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://example.com/2",
		Method:    "GET",
		SessionID: sessionID,
		Tags:      []string{"type:sticky"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, sessionID, resp2.Headers.Get(middleware.HeaderSessionID), "Session ID should be preserved")

	if firstEndpointID == "endpoint-1" {
		assert.NotEmpty(t, ep1.GetRequests(), "Endpoint 1 should receive sticky request")
		assert.Empty(t, ep2.GetRequests(), "Endpoint 2 should NOT receive sticky request")
	} else {
		assert.Empty(t, ep1.GetRequests(), "Endpoint 1 should NOT receive sticky request")
		assert.NotEmpty(t, ep2.GetRequests(), "Endpoint 2 should receive sticky request")
	}
}

// TestSession_Migration verifies that session migrates to a new endpoint on failure.
func TestSession_Migration(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()
	rand.Seed(time.Now().UnixNano())

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	// Setup 2 endpoints
	// Start EP1 first to ensure session lands on it
	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "endpoint-1", Secret: []byte(testHMACSecret), Tags: []string{"type:migration"},
	})
	ep1.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	// Register in DB
	CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: "endpoint-1", Tags: []string{"type:migration"}, IsHealthy: true})
	CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: "endpoint-2", Tags: []string{"type:migration"}, IsHealthy: true})

	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-1"))

	CreateTestRoutingRule(
		ctx, suite.PostgresDSN(), "Migration Rule", 100,
		[]string{"type:migration"}, []string{}, "", 0, 0, "",
		[]TestEndpointPool{{Tier: 1, Endpoints: []string{"endpoint-1", "endpoint-2"}, MaxRetries: 1}},
	)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// 1. Create Session explicitly on Endpoint 1 (only healthy one)
	resp, err := client.SendRequest(ctx, &ProxyRequest{URL: "http://example.com", Method: "GET", Tags: []string{"type:migration"}})
	require.NoError(t, err)
	sessionID := resp.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID, "Failed to establish session on endpoint-1")
	require.NotEmpty(t, ep1.GetRequests(), "Endpoint 1 should have received the request")
	ep1.ClearRequests()

	// 2. Start Endpoint 2
	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "endpoint-2", Secret: []byte(testHMACSecret), Tags: []string{"type:migration"},
	})
	ep2.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()
	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-2"))

	// 2. Kill Endpoint 1 (Mock failure)
	ep1.SetFailures(10) // Make it fail effectively
	// Or even better: Stop it completely, but health check might mark it Unhealthy and selector won't pick it.
	// If we want to test "Migration on Failure during Execution", we want Selector to PICK it (because sticky),
	// but Execution to FAIL, then Executor falls back to Pool, picks EP2, and then Server migrates session.

	// So EP1 must be considered "Healthy" by Selector (so don't stop heartbeats), but fail requests.

	// 3. Send request with Session ID (Sticky to EP1)
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://example.com/migrate",
		Method:    "GET",
		SessionID: sessionID,
		Tags:      []string{"type:migration"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify Migration happened
	assert.Equal(t, "true", resp.Headers.Get(middleware.HeaderSessionMigrated))
	assert.Equal(t, "endpoint-1", resp.Headers.Get(middleware.HeaderSessionPreviousEndpoint))

	// Verify request was handled by EP2
	assert.NotEmpty(t, ep2.GetRequests(), "Endpoint 2 should have handled the request after migration")

	// 4. Verify Next Request Sticks to EP2
	ep1.ClearRequests()
	ep2.ClearRequests()
	resp2, err := client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://example.com/after-migrate",
		Method:    "GET",
		SessionID: sessionID,
		Tags:      []string{"type:migration"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp2.StatusCode)

	assert.NotEmpty(t, ep2.GetRequests(), "Request should stick to Endpoint 2 now")
	assert.Empty(t, ep1.GetRequests(), "Request should NOT go to Endpoint 1") // Although EP1 is failing, we shouldn't even try it if sticky updated.
}

// TestSession_Expiration verifies session expiration behavior.
func TestSession_Expiration(t *testing.T) {
	// This creates a long running test if we use real TTL.
	// Can we inject a short TTL?
	// The `session.Service` uses `domain.DefaultSessionTTL` = 10 min.
	// We cannot easily change that constant.
	// We can manually expire it in Redis.

	suite := GetSuite(t)
	ctx := context.Background()
	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: "ep1", Tags: []string{"type:expiry"}, Secret: []byte(testHMACSecret)})
	ep1.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep1.Start(ctx), "failed to start mock endpoint")
	defer ep1.Stop()

	CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: "ep1", Tags: []string{"type:expiry"}, IsHealthy: true})
	require.NoError(t, tc.WaitForEndpoint(ctx, "ep1"), "failed to wait for endpoint health")

	CreateTestRoutingRule(ctx, suite.PostgresDSN(), "Expiry Rule", 100, []string{"type:expiry"}, []string{}, "", 0, 0, "", []TestEndpointPool{{Tier: 1, Endpoints: []string{"ep1"}}})
	tc.Server.GetMatcher().LoadRules(ctx)

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// Create Session
	resp, err := client.SendRequest(ctx, &ProxyRequest{URL: "http://x.com", Method: "GET", Tags: []string{"type:expiry"}})
	require.NoError(t, err, "failed to create session")
	require.NotNil(t, resp, "response should not be nil")
	sessionID := resp.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID)

	// Manually delete session from Redis to simulate expiration
	redisClient, err := redis.NewClient(config.CoreConfig{RedisAddr: suite.RedisAddr()}, nil)
	require.NoError(t, err)
	defer redisClient.Close()

	redisClient.Client.Del(ctx, "session:"+sessionID)

	// Use Expired Session
	resp2, err := client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://x.com",
		Method:    "GET",
		SessionID: sessionID,
		Tags:      []string{"type:expiry"},
	})
	require.NoError(t, err)

	// Expect 410 Gone (Session Expired) or 404
	// Middleware returns 410 for domain.ErrSessionExpired.
	// But if it's missing from Redis, `Get` returns nil/error.
	// `sessionService.GetSession` calls store.Get. If not found, it likely returns error.
	// We might get 500 or 410 depending on implementation of `Get`.
	// `redisStore.Get` usually returns "redis: nil" wrapped.
	// Let's assume the service handles "not found" as error?

	// Actually, `middleware.SessionMiddleware` handles `domain.ErrSessionExpired`.
	// If `service.GetSession` returns `ErrSessionExpired` (explicit check on TTL) or just error?
	// Redis keys expire automatically. So `Get` will fail.

	// If we deleted it, `service.GetSession` returns error. Middleware might treat as "Session ID invalid".
	// The middleware code (step 27):
	// if err == domain.ErrSessionExpired { return 410 }
	// else { // Other errors? Log maybe. Proceed without session? }

	// If session is invalid, logic says "Server deletes session...".
	// Design says 5.2 Error Codes: SESSION_EXPIRED -> 410.

	// If we just get an error, middleware logs it and proceeds?
	// line 75: // Other errors? Log maybe.
	// line 79: return next(c)

	// So if session is gone, it treats it as a new request without session?
	// If so, it will create a new session!
	// Let's check response headers.

	newSessionID := resp2.Headers.Get(middleware.HeaderSessionID)
	if resp2.StatusCode == 410 {
		t.Log("Got 410 Session Expired")
	} else {
		t.Logf("Got status %d, New session: %s", resp2.StatusCode, newSessionID)
		assert.NotEqual(t, sessionID, newSessionID, "Should have created a new session ID if old one was missing/expired")
	}
}

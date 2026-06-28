package integration

import (
	"context"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_Stickiness(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	rand.Seed(time.Now().UnixNano())

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

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

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: "endpoint-1", Tags: []string{"type:sticky"}, IsHealthy: true})
	require.NoError(t, err)
	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: "endpoint-2", Tags: []string{"type:sticky"}, IsHealthy: true})
	require.NoError(t, err)

	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-1"))
	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-2"))

	err = CreateTestRoutingRule(
		ctx, suite.PostgresDSN(), "Sticky Rule", 100,
		[]string{"type:sticky"}, []string{}, "", 0, 0, "",
		[]TestEndpointPool{{Tier: 1, Endpoints: []string{"endpoint-1", "endpoint-2"}, MaxRetries: 1}},
	)
	require.NoError(t, err)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	resp1, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    "http://example.com",
		Method: "GET",
		Tags:   []string{"type:sticky"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	sessionID := resp1.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID, "Session ID should be returned")

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

func TestSession_Migration(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()
	rand.Seed(time.Now().UnixNano())

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "endpoint-1", Secret: []byte(testHMACSecret), Tags: []string{"type:migration"},
	})
	ep1.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

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

	resp, err := client.SendRequest(ctx, &ProxyRequest{URL: "http://example.com", Method: "GET", Tags: []string{"type:migration"}})
	require.NoError(t, err)
	sessionID := resp.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID, "Failed to establish session on endpoint-1")
	require.NotEmpty(t, ep1.GetRequests(), "Endpoint 1 should have received the request")
	ep1.ClearRequests()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "endpoint-2", Secret: []byte(testHMACSecret), Tags: []string{"type:migration"},
	})
	ep2.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()
	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-2"))

	ep1.SetFailures(10)

	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://example.com/migrate",
		Method:    "GET",
		SessionID: sessionID,
		Tags:      []string{"type:migration"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	assert.Equal(t, "true", resp.Headers.Get(middleware.HeaderSessionMigrated))
	assert.Equal(t, "endpoint-1", resp.Headers.Get(middleware.HeaderSessionPreviousEndpoint))

	assert.NotEmpty(t, ep2.GetRequests(), "Endpoint 2 should have handled the request after migration")

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
	assert.Empty(t, ep1.GetRequests(), "Request should NOT go to Endpoint 1")
}

func TestSession_Expiration(t *testing.T) {

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

	resp, err := client.SendRequest(ctx, &ProxyRequest{URL: "http://x.com", Method: "GET", Tags: []string{"type:expiry"}})
	require.NoError(t, err, "failed to create session")
	require.NotNil(t, resp, "response should not be nil")
	sessionID := resp.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID)

	redisClient, err := redis.NewClient(config.RedisConfig{Addr: suite.RedisAddr()}, nil)
	require.NoError(t, err)
	defer redisClient.Close()

	redisClient.Client.Del(ctx, "session:"+sessionID)

	resp2, err := client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://x.com",
		Method:    "GET",
		SessionID: sessionID,
		Tags:      []string{"type:expiry"},
	})
	require.NoError(t, err)

	newSessionID := resp2.Headers.Get(middleware.HeaderSessionID)
	if resp2.StatusCode == http.StatusGone {
		t.Log("Got 410 Session Expired")
	} else {
		t.Logf("Got status %d, New session: %s", resp2.StatusCode, newSessionID)
		assert.NotEqual(t, sessionID, newSessionID, "Should have created a new session ID if old one was missing/expired")
	}
}

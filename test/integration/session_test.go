package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server/middleware"
)

func TestSession_Stickiness(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: testEndpoint1,
		Secret:     []byte(testHMACSecret),
		Tags:       []string{testTagSticky},
	})
	ep1.SetResponse(&MockEndpointResponse{
		StatusCode: 200, Body: []byte("Response from EP1"),
	})
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: testEndpoint2,
		Secret:     []byte(testHMACSecret),
		Tags:       []string{testTagSticky},
	})
	ep2.SetResponse(&MockEndpointResponse{
		StatusCode: 200, Body: []byte("Response from EP2"),
	})
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: testEndpoint1, Tags: []string{testTagSticky}, IsHealthy: true})
	require.NoError(t, err)
	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: testEndpoint2, Tags: []string{testTagSticky}, IsHealthy: true})
	require.NoError(t, err)

	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-1"))
	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-2"))

	err = CreateTestRoutingRule(
		ctx, suite.PostgresDSN(), "Sticky Rule", 100,
		[]string{testTagSticky}, []string{}, "", 0, 0, "",
		[]TestEndpointPool{{Tier: 1, Endpoints: []string{testEndpoint1, testEndpoint2}, MaxRetries: 1}},
	)
	require.NoError(t, err)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	resp1, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    "http://example.com",
		Method: httpGet,
		Tags:   []string{testTagSticky},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	sessionID := resp1.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID, "Session ID should be returned")

	var firstEndpointID string
	switch {
	case len(ep1.GetRequests()) > 0:
		firstEndpointID = "endpoint-1"
	case len(ep2.GetRequests()) > 0:
		firstEndpointID = "endpoint-2"
	default:
		t.Fatal("Neither endpoint received the request")
	}
	t.Logf("First request went to: %s", firstEndpointID)

	ep1.ClearRequests()
	ep2.ClearRequests()

	resp2, err := client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://example.com/2",
		Method:    httpGet,
		SessionID: sessionID,
		Tags:      []string{testTagSticky},
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

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: testEndpoint1, Secret: []byte(testHMACSecret), Tags: []string{testTagMigration},
	})
	ep1.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: testEndpoint1, Tags: []string{testTagMigration}, IsHealthy: true}))
	require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: testEndpoint2, Tags: []string{testTagMigration}, IsHealthy: true}))

	require.NoError(t, tc.WaitForEndpoint(ctx, "endpoint-1"))

	require.NoError(t, CreateTestRoutingRule(
		ctx, suite.PostgresDSN(), "Migration Rule", 100,
		[]string{testTagMigration}, []string{}, "", 0, 0, "",
		[]TestEndpointPool{{Tier: 1, Endpoints: []string{testEndpoint1, testEndpoint2}, MaxRetries: 1}},
	))
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	resp, err := client.SendRequest(ctx, &ProxyRequest{URL: "http://example.com", Method: httpGet, Tags: []string{testTagMigration}})
	require.NoError(t, err)
	sessionID := resp.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID, "Failed to establish session on endpoint-1")
	require.NotEmpty(t, ep1.GetRequests(), "Endpoint 1 should have received the request")
	ep1.ClearRequests()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: testEndpoint2, Secret: []byte(testHMACSecret), Tags: []string{testTagMigration},
	})
	ep2.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()
	require.NoError(t, tc.WaitForEndpoint(ctx, testEndpoint2))

	ep1.SetFailures(10)

	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://example.com/migrate",
		Method:    httpGet,
		SessionID: sessionID,
		Tags:      []string{testTagMigration},
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
		Method:    httpGet,
		SessionID: sessionID,
		Tags:      []string{testTagMigration},
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

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: testEp1, Tags: []string{testTagExpiry}, Secret: []byte(testHMACSecret)})
	ep1.SetResponse(&MockEndpointResponse{StatusCode: 200})
	require.NoError(t, ep1.Start(ctx), "failed to start mock endpoint")
	defer ep1.Stop()

	require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: testEp1, Tags: []string{testTagExpiry}, IsHealthy: true}))
	require.NoError(t, tc.WaitForEndpoint(ctx, testEp1), "failed to wait for endpoint health")

	require.NoError(t, CreateTestRoutingRule(ctx, suite.PostgresDSN(), "Expiry Rule", 100, []string{testTagExpiry}, []string{}, "", 0, 0, "", []TestEndpointPool{{Tier: 1, Endpoints: []string{testEp1}}}))
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	resp, err := client.SendRequest(ctx, &ProxyRequest{URL: "http://x.com", Method: httpGet, Tags: []string{testTagExpiry}})
	require.NoError(t, err, "failed to create session")
	require.NotNil(t, resp, "response should not be nil")
	sessionID := resp.Headers.Get(middleware.HeaderSessionID)
	require.NotEmpty(t, sessionID)

	redisClient, err := redis.NewClient(ctx, config.RedisConfig{Addr: suite.RedisAddr()}, nil)
	require.NoError(t, err)
	defer func() { _ = redisClient.Close() }()

	redisClient.Client.Del(ctx, "session:"+sessionID)

	resp2, err := client.SendRequest(ctx, &ProxyRequest{
		URL:       "http://x.com",
		Method:    httpGet,
		SessionID: sessionID,
		Tags:      []string{testTagExpiry},
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

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimit_PerSecond(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	// Create test data
	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	// Create rule with Limit 2/sec
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"limit-2-sec",
		100,
		[]string{},
		[]string{},
		"quota-sec",
		2,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	// Start mock endpoint
	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// Send 2 requests - expect success
	for i := 0; i < 2; i++ {
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:    tc.MockTarget.URL(),
			Method: "GET",
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
	}

	// Send 3rd request - expect 429
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "3rd request should be limited")
	assert.NotEmpty(t, resp.Headers.Get("Retry-After"), "Retry-After header should be present")

	// Wait for window to reset (slightly more than 1 sec to be safe)
	time.Sleep(1100 * time.Millisecond)

	// Send request - expect success
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "request after wait should succeed")
}

func TestRateLimit_PerMinute(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	// Create rule with Limit 3/min
	// Note: PerSecond is optional (0), so only PerMinute applies
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"limit-3-min",
		100,
		[]string{},
		[]string{},
		"quota-min",
		0,
		3,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// Send 3 requests - expect success
	for i := 0; i < 3; i++ {
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:    tc.MockTarget.URL(),
			Method: "GET",
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
	}

	// Send 4th request - expect 429
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "4th request should be limited")
}

func TestRateLimit_Headers(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	// Create rule with Limit 10/min (using per-minute to avoid race conditions in CI
	// where the 1-second window can reset during test execution)
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"limit-headers",
		100,
		[]string{},
		[]string{},
		"quota-headers",
		0,
		10,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// Send 10 requests to exhaust limit
	for i := 0; i < 10; i++ {
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:    tc.MockTarget.URL(),
			Method: "GET",
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// Send 11th request - expect 429 and Headers
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	// Check headers
	// We expect X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset
	limit := resp.Headers.Get("X-RateLimit-Limit")
	remaining := resp.Headers.Get("X-RateLimit-Remaining")
	reset := resp.Headers.Get("X-RateLimit-Reset")

	assert.NotEmpty(t, limit, "X-RateLimit-Limit header missing")
	assert.NotEmpty(t, remaining, "X-RateLimit-Remaining header missing")
	assert.NotEmpty(t, reset, "X-RateLimit-Reset header missing")

	// limit should be 10
	assert.Equal(t, "10", limit)

	// remaining should be 0
	assert.Equal(t, "0", remaining)
}

func TestRateLimit_QuotaIsolation(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test", "pool:a", "pool:b"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	// Rule A: Limit 1/sec, QuotaKey "key_a", RequiredTag "pool:a"
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"rule-a",
		100,
		[]string{"pool:a"},
		[]string{},
		"key_a",
		1,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	// Rule B: Limit 10/sec, QuotaKey "key_b", RequiredTag "pool:b"
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"rule-b",
		100,
		[]string{"pool:b"},
		[]string{},
		"key_b",
		10,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	// Re-create mock endpoint with tags to match rules
	tc.ReplaceMockEndpoint(NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "test-endpoint-1",
		Secret:     []byte(testHMACSecret),
		TargetURL:  tc.MockTarget.URL(),
		Tags:       []string{"type:test", "pool:a", "pool:b"},
	}))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))
	tc.MockTarget.SetDefaultResponse(MockTargetConfig{StatusCode: http.StatusOK, Body: []byte("OK")})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// 1. Send request to Rule A (OK)
	t.Log("Sending Request 1 to Rule A")
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: "GET",
		Tags:   []string{"pool:a"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Request 1 (Rule A) should succeed")

	// 2. Send request to Rule A (Limit exceeded)
	t.Log("Sending Request 2 to Rule A")
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: "GET",
		Tags:   []string{"pool:a"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Request 2 (Rule A) should be limited")

	// 3. Send request to Rule B (Should be OK, unaffected by A)
	t.Log("Sending Request 3 to Rule B")
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: "GET",
		Tags:   []string{"pool:b"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Rule B should not be affected by Rule A's limit")
}

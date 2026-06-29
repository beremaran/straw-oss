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

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        testEndpointID,
		Tags:      []string{testTagTest},
		IsHealthy: true,
	})
	require.NoError(t, err)

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
			{Tier: 1, Endpoints: []string{testEndpointID}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, testEndpointID))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	for i := range 2 {
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:    tc.MockTarget.URL(),
			Method: httpGet,
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
	}

	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: httpGet,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "3rd request should be limited")
	assert.NotEmpty(t, resp.Headers.Get("Retry-After"), "Retry-After header should be present")

	time.Sleep(1100 * time.Millisecond)

	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: httpGet,
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
		ID:        testEndpointID,
		Tags:      []string{testTagTest},
		IsHealthy: true,
	})
	require.NoError(t, err)

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
			{Tier: 1, Endpoints: []string{testEndpointID}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, testEndpointID))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	for i := range 3 {
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:    tc.MockTarget.URL(),
			Method: httpGet,
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
	}

	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: httpGet,
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
		ID:        testEndpointID,
		Tags:      []string{testTagTest},
		IsHealthy: true,
	})
	require.NoError(t, err)

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
			{Tier: 1, Endpoints: []string{testEndpointID}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, testEndpointID))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	for range 10 {
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:    tc.MockTarget.URL(),
			Method: httpGet,
		})
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: httpGet,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	limit := resp.Headers.Get("X-RateLimit-Limit")
	remaining := resp.Headers.Get("X-RateLimit-Remaining")
	reset := resp.Headers.Get("X-RateLimit-Reset")

	assert.NotEmpty(t, limit, "X-RateLimit-Limit header missing")
	assert.NotEmpty(t, remaining, "X-RateLimit-Remaining header missing")
	assert.NotEmpty(t, reset, "X-RateLimit-Reset header missing")

	assert.Equal(t, "10", limit)

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
		ID:        testEndpointID,
		Tags:      []string{testTagTest, testPoolA, testPoolB},
		IsHealthy: true,
	})
	require.NoError(t, err)

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"rule-a",
		100,
		[]string{testPoolA},
		[]string{},
		"key_a",
		1,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{testEndpointID}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"rule-b",
		100,
		[]string{testPoolB},
		[]string{},
		"key_b",
		10,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{testEndpointID}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	tc.ReplaceMockEndpoint(NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: testEndpointID,
		Secret:     []byte(testHMACSecret),
		TargetURL:  tc.MockTarget.URL(),
		Tags:       []string{testTagTest, testPoolA, testPoolB},
	}))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, testEndpointID))
	tc.MockTarget.SetDefaultResponse(MockTargetConfig{StatusCode: http.StatusOK, Body: []byte("OK")})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	t.Log("Sending Request 1 to Rule A")
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: httpGet,
		Tags:   []string{testPoolA},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Request 1 (Rule A) should succeed")

	t.Log("Sending Request 2 to Rule A")
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: httpGet,
		Tags:   []string{testPoolA},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "Request 2 (Rule A) should be limited")

	t.Log("Sending Request 3 to Rule B")
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL(),
		Method: httpGet,
		Tags:   []string{testPoolB},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Rule B should not be affected by Rule A's limit")
}

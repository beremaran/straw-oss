package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetry_SamePool(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "retry-client", []string{"*"})
	require.NoError(t, err)

	epIDs := []string{"same-pool-1", "same-pool-2", "same-pool-3"}
	for _, id := range epIDs {
		require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
			ID:        id,
			Tags:      []string{testRetrySamePool},
			IsHealthy: true,
		}))
	}

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"retry-same-pool-rule",
		100,
		[]string{testRetrySamePool},
		[]string{},
		"",
		0,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: epIDs, MaxRetries: 3},
		},
	)
	require.NoError(t, err)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: "same-pool-1", Secret: []byte(testHMACSecret), Tags: []string{testRetrySamePool}})
	ep1.SetFailures(10)
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: "same-pool-2", Secret: []byte(testHMACSecret), Tags: []string{testRetrySamePool}})
	ep2.SetFailures(10)
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()

	ep3 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: "same-pool-3", Secret: []byte(testHMACSecret), Tags: []string{testRetrySamePool}})
	require.NoError(t, ep3.Start(ctx))
	defer ep3.Stop()

	require.NoError(t, tc.WaitForEndpoint(ctx, "same-pool-1"))
	require.NoError(t, tc.WaitForEndpoint(ctx, "same-pool-2"))
	require.NoError(t, tc.WaitForEndpoint(ctx, "same-pool-3"))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/retry-me",
		Method: httpGet,
		Tags:   []string{testRetrySamePool},
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRetry_Escalation(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "escalation-client", []string{"*"})
	require.NoError(t, err)

	require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID: testEpTier1, Tags: []string{"tier:1"}, IsHealthy: true,
	}))
	require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID: testEpTier2, Tags: []string{"tier:2"}, IsHealthy: true,
	}))

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"escalation-rule",
		100,
		[]string{testModeEscalation},
		[]string{},
		"",
		0,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{testEpTier1}, MaxRetries: 1},
			{Tier: 2, Endpoints: []string{testEpTier2}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: testEpTier1,
		Secret:     []byte(testHMACSecret),
		Tags:       []string{"tier:1", testModeEscalation},
	})
	ep1.SetFailures(100)
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: testEpTier2,
		Secret:     []byte(testHMACSecret),
		Tags:       []string{"tier:2", testModeEscalation},
	})
	ep2.SetResponse(&MockEndpointResponse{
		StatusCode: 200,
		Body:       []byte("Success from Tier 2"),
	})
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()

	require.NoError(t, tc.WaitForEndpoint(ctx, testEpTier1))
	require.NoError(t, tc.WaitForEndpoint(ctx, testEpTier2))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/escalate",
		Method: httpGet,
		Tags:   []string{testModeEscalation},
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(resp.Body), "Tier 2")

	assert.GreaterOrEqual(t, ep1.RequestCount(), 1)
	assert.Equal(t, 1, ep2.RequestCount())

	if pool := resp.Headers.Get("X-Relay-Pool"); pool != "" {
		assert.Equal(t, "2", pool)
	}
}

func TestRetry_ImmediateEscalation_403(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "blocked-client", []string{"*"})
	require.NoError(t, err)

	require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: testEpBlocked, Tags: []string{"tier:blocked"}, IsHealthy: true}))
	require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: testEpBackup, Tags: []string{"tier:backup"}, IsHealthy: true}))

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"blocked-rule",
		100,
		[]string{testModeBlocked},
		[]string{},
		"",
		0,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{testEpBlocked}, MaxRetries: 3},
			{Tier: 2, Endpoints: []string{testEpBackup}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: testEpBlocked, Secret: []byte(testHMACSecret), Tags: []string{"tier:blocked", testModeBlocked}})
	ep1.SetResponse(&MockEndpointResponse{
		StatusCode: http.StatusForbidden,
		Body:       []byte("Blocked by target"),
	})
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: testEpBackup, Secret: []byte(testHMACSecret), Tags: []string{"tier:backup", testModeBlocked}})
	ep2.SetResponse(&MockEndpointResponse{StatusCode: 200, Body: []byte("Backup Success")})
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()

	require.NoError(t, tc.WaitForEndpoint(ctx, testEpBlocked))
	require.NoError(t, tc.WaitForEndpoint(ctx, testEpBackup))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/blocked",
		Method: httpGet,
		Tags:   []string{testModeBlocked},
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(resp.Body), "Backup Success")

	assert.Equal(t, 1, ep1.RequestCount())
}

func TestRetry_Exhaustion_503(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "exhaust-client", []string{"*"})
	require.NoError(t, err)

	epIDs := []string{"exhaust-1", "exhaust-2"}
	for _, id := range epIDs {
		require.NoError(t, CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{ID: id, Tags: []string{testRetryExhaust}, IsHealthy: true}))
	}

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"exhaust-rule",
		100,
		[]string{testRetryExhaust},
		[]string{},
		"",
		0,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: epIDs, MaxRetries: 2},
		},
	)
	require.NoError(t, err)
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	ep1 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: "exhaust-1", Secret: []byte(testHMACSecret), Tags: []string{testRetryExhaust}})
	ep1.SetFailures(10)
	require.NoError(t, ep1.Start(ctx))
	defer ep1.Stop()

	ep2 := NewMockEndpoint(tc.Broker, MockEndpointConfig{EndpointID: "exhaust-2", Secret: []byte(testHMACSecret), Tags: []string{testRetryExhaust}})
	ep2.SetFailures(10)
	require.NoError(t, ep2.Start(ctx))
	defer ep2.Stop()

	require.NoError(t, tc.WaitForEndpoint(ctx, "exhaust-1"))
	require.NoError(t, tc.WaitForEndpoint(ctx, "exhaust-2"))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/exhaust",
		Method: httpGet,
		Tags:   []string{testRetryExhaust},
	})
	require.NoError(t, err)

	assert.Contains(t, []int{http.StatusServiceUnavailable, http.StatusBadGateway}, resp.StatusCode)

	assert.Equal(t, 2, ep1.RequestCount()+ep2.RequestCount())
}

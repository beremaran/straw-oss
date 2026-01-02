// Package integration provides testcontainer-based infrastructure for integration testing.
package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/broker"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/infra/circuitbreaker"
	"github.com/kwilabs/straw-proxy-server/internal/infra/postgres"
	infraredis "github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/kwilabs/straw-proxy-server/internal/server"
	"github.com/kwilabs/straw-proxy-server/internal/service/auth"
	"github.com/kwilabs/straw-proxy-server/internal/service/endpoint"
	"github.com/kwilabs/straw-proxy-server/internal/service/filter"
	"github.com/kwilabs/straw-proxy-server/internal/service/orchestrator"
	"github.com/kwilabs/straw-proxy-server/internal/service/ratelimit"
	"github.com/kwilabs/straw-proxy-server/internal/service/router"
	"github.com/kwilabs/straw-proxy-server/internal/service/session"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHMACSecret = "test-hmac-secret-for-integration-tests"

// testServerContext holds a running server instance with all dependencies.
type testServerContext struct {
	Server        *server.Server
	ServerURL     string
	httpServer    *httptest.Server
	Broker        *broker.NatsBroker
	MockEndpoint  *MockEndpoint
	MockTarget    *MockTargetServer
	HealthService *endpoint.HealthService
	HealthStore   *infraredis.EndpointHealthStore
	Cleanup       func()
}

// setupTestServer creates a fully wired server for E2E testing.
func setupTestServer(t *testing.T, suite *TestSuite) *testServerContext {
	t.Helper()
	ctx := context.Background()

	// Clean the database for this test
	suite.CleanupForTest(t)

	// Connect to infrastructure
	broker := broker.NewNatsBroker(broker.Addrs(suite.NatsURL()))
	err := broker.Connect()
	require.NoError(t, err, "failed to connect to NATS")

	// Declare the heartbeats logic
	// In NATS initialization we use DeclareExchange to create Streams.
	// "heartbeats" -> fanout behavior
	err = broker.DeclareExchange(ctx, "heartbeats", "fanout")
	require.NoError(t, err, "failed to declare heartbeats exchange")

	err = broker.DeclareQueue(ctx, "heartbeats")
	require.NoError(t, err, "failed to declare heartbeats queue")

	err = broker.BindQueue(ctx, "heartbeats", "heartbeats", "")
	require.NoError(t, err, "failed to bind heartbeats queue")

	err = broker.DeclareExchange(ctx, "tasks", "direct")
	require.NoError(t, err, "failed to declare tasks exchange")

	err = broker.DeclareExchange(ctx, "results", "topic")
	require.NoError(t, err, "failed to declare results exchange")

	redisClient, err := infraredis.NewClient(suite.RedisAddr(), nil)
	require.NoError(t, err, "failed to connect to Redis")

	// Create postgres client and repositories
	postgresClient, err := postgres.NewClient(ctx, suite.PostgresDSN(), nil)
	require.NoError(t, err, "failed to create postgres client")

	apiKeyRepo := postgres.NewApiKeyRepository(postgresClient)
	ruleRepo := postgres.NewRoutingRuleRepository(postgresClient)

	// Create services
	keyCache := auth.NewAuthCache(redisClient, 5*time.Minute)
	authService := auth.NewAuthService(apiKeyRepo, keyCache)

	ruleCache := router.NewRuleCache(redisClient.Client, 5*time.Minute)
	routerMatcher := router.NewMatcher(ruleRepo, ruleCache)
	require.NoError(t, routerMatcher.LoadRules(ctx), "failed to load routing rules")

	rateLimiter := ratelimit.NewRateLimiter(redisClient)
	filterService := filter.NewService(nil) // No URL filter for simplicity
	sessionStore := session.NewRedisStore(redisClient)
	sessionService := session.NewService(sessionStore)

	// Create orchestrator components
	healthStore := infraredis.NewEndpointHealthStore(redisClient)
	selector := orchestrator.NewSimpleEndpointSelector(healthStore)
	circuitBreaker := circuitbreaker.New(circuitbreaker.Config{})
	publisher := orchestrator.NewPublisher(broker, selector, []byte(testHMACSecret), circuitBreaker)
	consumer := orchestrator.NewConsumer(broker)
	executor := orchestrator.NewRetryExecutor(publisher, consumer, selector, broker, []byte(testHMACSecret))

	// Declare and bind the shared results queue for the RetryExecutor
	err = broker.DeclareQueue(ctx, orchestrator.SharedResultQueue)
	require.NoError(t, err, "failed to declare shared results queue")

	// Bind shared results queue to simulate direct-to-queue (using queue name as subject due to empty exchange publish)
	err = broker.BindQueue(ctx, orchestrator.SharedResultQueue, "", orchestrator.SharedResultQueue)
	require.NoError(t, err, "failed to bind shared results queue")

	// Start the shared result queue consumer (required for response dispatching)
	err = executor.Start(ctx)
	require.NoError(t, err, "failed to start retry executor")

	// Start health service to consume heartbeats and register endpoints
	healthService := endpoint.NewHealthService(broker, healthStore)
	err = healthService.Start(ctx)
	require.NoError(t, err, "failed to start health service")

	// Create server configuration
	serverConfig := config.ServerConfig{
		Core: config.CoreConfig{
			LogLevel:  "debug",
			LogFormat: "json",
		},
		Security: config.SecurityConfig{
			HMACSecret: testHMACSecret,
		},
		HTTPPort:      0, // Random port
		ResultTimeout: 30 * time.Second,
		MaxBodySize:   "10M",
	}

	// Create server - allow private IPs for integration tests (mock targets run on localhost)
	srv := server.New(
		serverConfig,
		authService,
		sessionService,
		routerMatcher,
		rateLimiter,
		filterService,
		executor,
		server.WithAllowPrivateIPs(),
	)

	// Start the server in a test HTTP server
	testServer := httptest.NewServer(srv.GetEcho())

	// Create mock target server
	mockTarget := NewMockTargetServer()

	// Create mock endpoint with tags matching the routing rules
	mockEndpoint := NewMockEndpoint(broker, MockEndpointConfig{
		EndpointID: "test-endpoint-1",
		Secret:     []byte(testHMACSecret),
		TargetURL:  mockTarget.URL(),
		Tags:       []string{"type:test"}, // Tags must match routing rule requirements
	})

	cleanup := func() {
		healthService.Stop()
		mockEndpoint.Stop()
		mockTarget.Close()
		testServer.Close()
		broker.Close()
		redisClient.Close()
	}

	return &testServerContext{
		Server:        srv,
		ServerURL:     testServer.URL,
		httpServer:    testServer,
		Broker:        broker,
		MockEndpoint:  mockEndpoint,
		MockTarget:    mockTarget,
		HealthService: healthService,
		HealthStore:   healthStore,
		Cleanup:       cleanup,
	}
}

// WaitForEndpoint waits for an endpoint to become healthy.
func (tc *testServerContext) WaitForEndpoint(ctx context.Context, endpointID string) error {
	return WaitForHealthy(ctx, func() error {
		health, err := tc.HealthStore.GetHealth(ctx, endpointID)
		if err != nil {
			return err
		}
		if health.State != infraredis.HealthStateHealthy {
			return fmt.Errorf("endpoint %s is %s", endpointID, health.State)
		}
		return nil
	}, 100*time.Millisecond, 10*time.Second)
}

// TestRequestFlow_BasicRequest tests a basic request through the system.
func TestRequestFlow_BasicRequest(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	// Setup test server
	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	// Create test data
	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err, "failed to create API key")

	// Create endpoint record
	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test"},
		IsHealthy: true,
	})
	require.NoError(t, err, "failed to create endpoint")

	// Create catch-all routing rule
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"Catch All Rule",
		100,
		[]string{},
		[]string{},
		"",
		0,
		0,
		"chrome-133",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 2},
		},
	)
	require.NoError(t, err, "failed to create routing rule")

	// Reload routing rules
	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	// Start mock endpoint
	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err, "failed to start mock endpoint")
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"), "failed to wait for endpoint health")

	// Configure mock target response
	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       []byte("Hello from target!"),
	})

	// Make request through proxy
	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/test",
		Method: "GET",
	})
	require.NoError(t, err, "proxy request failed")

	// Verify response
	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK")
	assert.Contains(t, string(resp.Body), "Hello from target!", "response body should contain target response")

	// Verify mock endpoint received the request
	requests := tc.MockEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(requests), 1, "mock endpoint should have received at least one request")
	if len(requests) > 0 {
		assert.Equal(t, "GET", requests[0].Method)
		assert.Contains(t, requests[0].URL, "/test")
	}

	// Verify mock target received the request
	targetRequests := tc.MockTarget.GetRequests()
	assert.GreaterOrEqual(t, len(targetRequests), 1, "mock target should have received at least one request")
}

// TestRequestFlow_WithTags tests request routing with specific tags.
func TestRequestFlow_WithTags(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	// Create test data
	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test", "target:amazon"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	// Create two routing rules with different tags
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"Amazon Rule",
		200,
		[]string{"target:amazon"},
		[]string{},
		"amazon",
		0,
		0,
		"chrome-133",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 2},
		},
	)
	require.NoError(t, err)

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"Default Rule",
		100,
		[]string{},
		[]string{},
		"default",
		0,
		0,
		"firefox-130",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 2},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	// Re-create mock endpoint with amazon tag to ensure heartbeats match the rule
	tc.MockEndpoint = NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "test-endpoint-1",
		Secret:     []byte(testHMACSecret),
		TargetURL:  tc.MockTarget.URL(),
		Tags:       []string{"type:test", "target:amazon"},
	})

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	// Test request WITH amazon tag - should match amazon-rule
	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/product",
		Method: "GET",
		Tags:   []string{"target:amazon"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// The request should have used the amazon rule (fingerprint chrome-133)
	requests := tc.MockEndpoint.GetRequests()
	require.GreaterOrEqual(t, len(requests), 1)
	assert.Equal(t, "chrome-133", requests[0].Fingerprint, "should use chrome-133 fingerprint from amazon rule")

	tc.MockEndpoint.ClearRequests()

	// Test request WITHOUT amazon tag - should match default-rule
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/other",
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	requests = tc.MockEndpoint.GetRequests()
	require.GreaterOrEqual(t, len(requests), 1)
	assert.Equal(t, "firefox-130", requests[0].Fingerprint, "should use firefox-130 fingerprint from default rule")
}

// TestRequestFlow_WithSession tests sticky session routing.
func TestRequestFlow_WithSession(t *testing.T) {
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

	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"session-rule",
		100,
		[]string{},
		[]string{},
		"",
		0,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 2},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("Session test"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// First request - creates session
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/login",
		Method: "POST",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Extract session ID from response header
	sessionID := resp.Headers.Get("X-Session-ID")
	t.Logf("Session ID from first request: %s", sessionID)

	// Second request with session ID - should route to same endpoint
	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:       tc.MockTarget.URL() + "/profile",
		Method:    "GET",
		SessionID: sessionID,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify both requests went to the same endpoint
	requests := tc.MockEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(requests), 2, "should have received at least 2 requests")
}

// TestRequestFlow_RateLimitExceeded tests rate limiting behavior.
func TestRequestFlow_RateLimitExceeded(t *testing.T) {
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

	// Create routing rule with strict rate limit
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"rate-limited-rule",
		100,
		[]string{},
		[]string{},
		"test-quota",
		1,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"test-endpoint-1"}, MaxRetries: 2},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("Rate limit test"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// First request - should succeed
	resp1, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/api",
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode, "first request should succeed")

	// Second request immediately - should be rate limited
	resp2, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/api",
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusTooManyRequests, resp2.StatusCode, "second immediate request should be rate limited")

	// Check for Retry-After header
	retryAfter := resp2.Headers.Get("Retry-After")
	assert.NotEmpty(t, retryAfter, "should have Retry-After header")
	t.Logf("Retry-After: %s", retryAfter)

	// Wait and try again - should succeed
	time.Sleep(1100 * time.Millisecond)
	resp3, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/api",
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp3.StatusCode, "request after waiting should succeed")
}

// TestRequestFlow_RetryFallback tests retry and pool escalation behavior.
func TestRequestFlow_RetryFallback(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	// Create test data
	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	// Create two endpoints for different pools
	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "primary-endpoint",
		Tags:      []string{"type:primary"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "fallback-endpoint",
		Tags:      []string{"type:fallback"},
		IsHealthy: true,
	})
	require.NoError(t, err)

	// Create routing rule with tiered pools
	err = CreateTestRoutingRule(
		ctx,
		suite.PostgresDSN(),
		"retry-rule",
		100,
		[]string{},
		[]string{},
		"",
		0,
		0,
		"",
		[]TestEndpointPool{
			{Tier: 1, Endpoints: []string{"primary-endpoint"}, MaxRetries: 1},
			{Tier: 2, Endpoints: []string{"fallback-endpoint"}, MaxRetries: 1},
		},
	)
	require.NoError(t, err)

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	// Create primary endpoint that will fail
	primaryEndpoint := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "primary-endpoint",
		Secret:     []byte(testHMACSecret),
		Tags:       []string{"type:primary"},
	})
	primaryEndpoint.SetFailures(2) // Fail first 2 requests
	err = primaryEndpoint.Start(ctx)
	require.NoError(t, err)
	defer primaryEndpoint.Stop()
	require.NoError(t, tc.WaitForEndpoint(ctx, "primary-endpoint"))

	// Create fallback endpoint that will succeed
	fallbackEndpoint := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "fallback-endpoint",
		Secret:     []byte(testHMACSecret),
		Tags:       []string{"type:fallback"},
	})
	fallbackEndpoint.SetResponse(&MockEndpointResponse{
		StatusCode: http.StatusOK,
		Headers: protocol.HeaderMap{
			{Key: "Content-Type", Value: "text/plain"},
		},
		Body: []byte("Fallback succeeded!"),
	})
	err = fallbackEndpoint.Start(ctx)
	require.NoError(t, err)
	defer fallbackEndpoint.Stop()
	require.NoError(t, tc.WaitForEndpoint(ctx, "fallback-endpoint"))

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)

	// Make request - should retry and fall back to secondary pool
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    "http://example.com/test",
		Method: "GET",
	})
	require.NoError(t, err)

	// Should eventually succeed via fallback
	assert.Equal(t, http.StatusOK, resp.StatusCode, "request should succeed via fallback")
	assert.Contains(t, string(resp.Body), "Fallback", "response should come from fallback endpoint")

	// Check retry headers
	retries := resp.Headers.Get("X-Relay-Retries")
	pool := resp.Headers.Get("X-Relay-Pool")
	t.Logf("Retries: %s, Pool: %s", retries, pool)

	// Verify primary endpoint received requests (and failed them)
	primaryReqs := primaryEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(primaryReqs), 1, "primary endpoint should have received at least 1 request")

	// Verify fallback endpoint received request
	fallbackReqs := fallbackEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(fallbackReqs), 1, "fallback endpoint should have received the request")
}

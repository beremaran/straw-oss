package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/infra/circuitbreaker"
	"github.com/beremaran/straw/internal/infra/postgres"
	infraredis "github.com/beremaran/straw/internal/infra/redis"
	"github.com/beremaran/straw/internal/server"
	"github.com/beremaran/straw/internal/service/auth"
	"github.com/beremaran/straw/internal/service/endpoint"
	"github.com/beremaran/straw/internal/service/filter"
	"github.com/beremaran/straw/internal/service/orchestrator"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/beremaran/straw/internal/service/router"
	"github.com/beremaran/straw/internal/service/session"
	"github.com/beremaran/straw/pkg/broker"
	"github.com/beremaran/straw/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHMACSecret = "test-hmac-secret-for-integration-tests"

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

func (tc *testServerContext) ReplaceMockEndpoint(newEndpoint *MockEndpoint) {
	if tc.MockEndpoint != nil {
		tc.MockEndpoint.Stop()
	}
	tc.MockEndpoint = newEndpoint
}

func setupTestServer(t *testing.T, suite *TestSuite) *testServerContext {
	t.Helper()
	ctx := context.Background()

	suite.CleanupForTest(t)

	broker := broker.NewNatsBroker(broker.Addrs(suite.NatsURL()))
	err := broker.Connect()
	require.NoError(t, err, "failed to connect to NATS")

	err = broker.DeclareStream(ctx, "heartbeats", "heartbeats.>")
	require.NoError(t, err, "failed to declare heartbeats stream")

	err = broker.DeclareStream(ctx, "tasks", "tasks.>")
	require.NoError(t, err, "failed to declare tasks stream")

	err = broker.DeclareStream(ctx, "results", "results.>")
	require.NoError(t, err, "failed to declare results stream")

	redisClient, err := infraredis.NewClient(ctx, config.RedisConfig{Addr: suite.RedisAddr()}, nil)
	require.NoError(t, err, "failed to connect to Redis")

	postgresClient, err := postgres.NewClient(ctx, suite.PostgresDSN(), nil)
	require.NoError(t, err, "failed to create postgres client")

	apiKeyRepo := postgres.NewApiKeyRepository(postgresClient)
	apiKeyTokenRepo := postgres.NewApiKeyTokenRepository(postgresClient)
	ruleRepo := postgres.NewRoutingRuleRepository(postgresClient)

	keyCache := auth.NewAuthCache(redisClient, 5*time.Minute)
	authService := auth.NewAuthService(apiKeyRepo, apiKeyTokenRepo, keyCache)

	ruleCache := router.NewRuleCache(redisClient.Client, 5*time.Minute)
	routerMatcher := router.NewMatcher(ruleRepo, ruleCache)
	require.NoError(t, routerMatcher.LoadRules(ctx), "failed to load routing rules")

	rateLimiter := ratelimit.NewRateLimiter(redisClient)
	filterService := filter.NewService(nil)
	sessionStore := session.NewRedisStore(redisClient)
	sessionService := session.NewService(sessionStore)

	healthStore := infraredis.NewEndpointHealthStore(redisClient)
	selector := orchestrator.NewSimpleEndpointSelector(healthStore)
	circuitBreaker := circuitbreaker.New(circuitbreaker.Config{})
	publisher := orchestrator.NewPublisher(broker, selector, []byte(testHMACSecret), circuitBreaker)
	consumer := orchestrator.NewConsumer(broker)
	executor := orchestrator.NewRetryExecutor(publisher, consumer, selector, broker, []byte(testHMACSecret))

	err = executor.Start(ctx)
	require.NoError(t, err, "failed to start retry executor")

	healthService := endpoint.NewHealthService(broker, healthStore)
	err = healthService.Start(ctx)
	require.NoError(t, err, "failed to start health service")

	serverConfig := config.ServerConfig{
		Observability: config.ObservabilityConfig{
			LogLevel:  "debug",
			LogFormat: "json",
		},
		Security: config.SecurityConfig{
			HMACSecret: testHMACSecret,
		},
		HTTPPort:      0,
		ResultTimeout: 30 * time.Second,
		MaxBodySize:   "10M",
	}

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

	testServer := httptest.NewServer(srv.GetHandler())

	mockTarget := NewMockTargetServer()

	mockEndpoint := NewMockEndpoint(broker, MockEndpointConfig{
		EndpointID: "test-endpoint-1",
		Secret:     []byte(testHMACSecret),
		TargetURL:  mockTarget.URL(),
		Tags:       []string{"type:test"},
	})

	tc := &testServerContext{
		Server:        srv,
		ServerURL:     testServer.URL,
		httpServer:    testServer,
		Broker:        broker,
		MockEndpoint:  mockEndpoint,
		MockTarget:    mockTarget,
		HealthService: healthService,
		HealthStore:   healthStore,
	}

	tc.Cleanup = func() {
		healthService.Stop()
		if tc.MockEndpoint != nil {
			tc.MockEndpoint.Stop()
		}
		mockTarget.Close()
		testServer.Close()
		_ = broker.Close()
		_ = redisClient.Close()
	}

	return tc
}

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

func TestRequestFlow_BasicRequest(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err, "failed to create API key")

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test"},
		IsHealthy: true,
	})
	require.NoError(t, err, "failed to create endpoint")

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

	require.NoError(t, tc.Server.GetMatcher().LoadRules(ctx))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err, "failed to start mock endpoint")
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"), "failed to wait for endpoint health")

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       []byte("Hello from target!"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/test",
		Method: "GET",
	})
	require.NoError(t, err, "proxy request failed")

	assert.Equal(t, http.StatusOK, resp.StatusCode, "expected 200 OK")
	assert.Contains(t, string(resp.Body), "Hello from target!", "response body should contain target response")

	requests := tc.MockEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(requests), 1, "mock endpoint should have received at least one request")
	if len(requests) > 0 {
		assert.Equal(t, "GET", requests[0].Method)
		assert.Contains(t, requests[0].URL, "/test")
	}

	targetRequests := tc.MockTarget.GetRequests()
	assert.GreaterOrEqual(t, len(targetRequests), 1, "mock target should have received at least one request")
}

func TestRequestFlow_WithTags(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

	err = CreateTestEndpoint(ctx, suite.PostgresDSN(), &TestEndpoint{
		ID:        "test-endpoint-1",
		Tags:      []string{"type:test", "target:amazon"},
		IsHealthy: true,
	})
	require.NoError(t, err)

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

	tc.ReplaceMockEndpoint(NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "test-endpoint-1",
		Secret:     []byte(testHMACSecret),
		TargetURL:  tc.MockTarget.URL(),
		Tags:       []string{"type:test", "target:amazon"},
	}))

	err = tc.MockEndpoint.Start(ctx)
	require.NoError(t, err)
	require.NoError(t, tc.WaitForEndpoint(ctx, "test-endpoint-1"))

	tc.MockTarget.SetDefaultResponse(MockTargetConfig{
		StatusCode: http.StatusOK,
		Body:       []byte("OK"),
	})

	client := NewHTTPTestClient(tc.ServerURL, apiKey.RawKey)
	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/product",
		Method: "GET",
		Tags:   []string{"target:amazon"},
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	requests := tc.MockEndpoint.GetRequests()
	require.GreaterOrEqual(t, len(requests), 1)
	assert.Equal(t, "chrome-133", requests[0].Fingerprint, "should use chrome-133 fingerprint from amazon rule")

	tc.MockEndpoint.ClearRequests()

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

func TestRequestFlow_WithSession(t *testing.T) {
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

	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/login",
		Method: "POST",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	sessionID := resp.Headers.Get("X-Session-ID")
	t.Logf("Session ID from first request: %s", sessionID)

	resp, err = client.SendRequest(ctx, &ProxyRequest{
		URL:       tc.MockTarget.URL() + "/profile",
		Method:    "GET",
		SessionID: sessionID,
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	requests := tc.MockEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(requests), 2, "should have received at least 2 requests")
}

func TestRequestFlow_RateLimitExceeded(t *testing.T) {
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

	resp1, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/api",
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp1.StatusCode, "first request should succeed")

	var resp2 *ProxyResponse
	for i := 0; i < 20; i++ {
		resp, err := client.SendRequest(ctx, &ProxyRequest{
			URL:    tc.MockTarget.URL() + "/api",
			Method: "GET",
		})
		require.NoError(t, err)
		if resp.StatusCode == http.StatusTooManyRequests {
			resp2 = resp

			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.NotNil(t, resp2, "should have been rate limited")
	assert.Equal(t, http.StatusTooManyRequests, resp2.StatusCode, "second immediate request should be rate limited")

	retryAfter := resp2.Headers.Get("Retry-After")
	assert.NotEmpty(t, retryAfter, "should have Retry-After header")
	t.Logf("Retry-After: %s", retryAfter)

	time.Sleep(1100 * time.Millisecond)
	resp3, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    tc.MockTarget.URL() + "/api",
		Method: "GET",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp3.StatusCode, "request after waiting should succeed")
}

func TestRequestFlow_RetryFallback(t *testing.T) {
	suite := GetSuite(t)
	ctx := context.Background()

	tc := setupTestServer(t, suite)
	defer tc.Cleanup()

	apiKey, err := CreateTestAPIKey(ctx, suite.PostgresDSN(), "test-client", []string{"*"})
	require.NoError(t, err)

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

	primaryEndpoint := NewMockEndpoint(tc.Broker, MockEndpointConfig{
		EndpointID: "primary-endpoint",
		Secret:     []byte(testHMACSecret),
		Tags:       []string{"type:primary"},
	})
	primaryEndpoint.SetFailures(2)
	err = primaryEndpoint.Start(ctx)
	require.NoError(t, err)
	defer primaryEndpoint.Stop()
	require.NoError(t, tc.WaitForEndpoint(ctx, "primary-endpoint"))

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

	resp, err := client.SendRequest(ctx, &ProxyRequest{
		URL:    "http://example.com/test",
		Method: "GET",
	})
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "request should succeed via fallback")
	assert.Contains(t, string(resp.Body), "Fallback", "response should come from fallback endpoint")

	retries := resp.Headers.Get("X-Relay-Retries")
	pool := resp.Headers.Get("X-Relay-Pool")
	t.Logf("Retries: %s, Pool: %s", retries, pool)

	primaryReqs := primaryEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(primaryReqs), 1, "primary endpoint should have received at least 1 request")

	fallbackReqs := fallbackEndpoint.GetRequests()
	assert.GreaterOrEqual(t, len(fallbackReqs), 1, "fallback endpoint should have received the request")
}

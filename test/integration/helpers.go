// Package integration provides test helpers for integration tests.
package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib" // register pgx driver
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/infra/redis"
)

const (
	testShutdownTimeout  = 5 * time.Second
	testResultTimeout    = 30 * time.Second
	testConcurrencyLimit = 100
	testMaxPoolHosts     = 10
	testIdleConnsPerHost = 2
	testIdleConnTimeout  = 90 * time.Second
	testHTTPTimeout      = 30 * time.Second
)

const (
	testEndpointID = "test-endpoint-1"
	testTagTest    = "type:test"
	httpGet        = "GET"
	testPoolA      = "pool:a"
	testPoolB      = "pool:b"

	testLogLevel              = "debug"
	testLogFormat             = "json"
	testEndpoint1             = "endpoint-1"
	testEndpoint2             = "endpoint-2"
	testTagSticky             = "type:sticky"
	testTagMigration          = "type:migration"
	testTagExpiry             = "type:expiry"
	testTagPrimary            = "type:primary"
	testTagFallback           = "type:fallback"
	testEndpointPrimary       = "primary-endpoint"
	testEndpointFallback      = "fallback-endpoint"
	testRetrySamePool         = "retry:same-pool"
	testRetryExhaust          = "retry:exhaust"
	testEpTier1               = "ep-tier-1"
	testEpTier2               = "ep-tier-2"
	testModeEscalation        = "mode:escalation"
	testEpBlocked             = "ep-blocked"
	testEpBackup              = "ep-backup"
	testModeBlocked           = "mode:blocked"
	testTargetAmazon          = "target:amazon"
	testEp1                   = "ep1"
	httpContentType           = "Content-Type"
	httpContentTypeJSON       = "application/json"
	testHTTPHeaderContentType = "Content-Type"
	textPlain                 = "text/plain"

	tableAPIKeys            = "api_keys"
	tableRoutingRules       = "routing_rules"
	tableEndpoints          = "endpoints"
	tableAuditLog           = "audit_log"
	tableUsageRecords       = "usage_records"
	tableCostMultipliers    = "cost_multipliers"
	tableUsageDailySummary  = "usage_daily_summary"
	tableAdminAuditLog      = "admin_audit_log"
	tableFingerprintPresets = "fingerprint_presets"
)

// ErrHealthCheckTimeout is returned when a health check times out.
var ErrHealthCheckTimeout = errors.New("health check timed out")

// NewTestServerConfig returns a test server configuration.
func NewTestServerConfig(postgresDSN, redisAddr, natsURL string) *config.ServerConfig {
	return &config.ServerConfig{
		Database: config.DatabaseConfig{
			DSN:         postgresDSN,
			AutoMigrate: false,
		},
		Redis: config.RedisConfig{
			Addr: redisAddr,
		},
		NATS: config.NATSConfig{
			URL: natsURL,
		},
		Security: config.SecurityConfig{
			HMACSecret: "test-hmac-secret-for-integration-tests",
		},
		Observability: config.ObservabilityConfig{
			LogLevel:       "debug",
			LogFormat:      "json",
			MetricsEnabled: false,
		},
		HTTPPort:         0,
		ManagementPort:   0,
		ShutdownTimeout:  testShutdownTimeout,
		ResultTimeout:    testResultTimeout,
		ManagementAPIKey: "test-management-api-key",
		MaxBodySize:      "2M",
	}
}

// NewTestEndpointConfig returns a test endpoint configuration.
func NewTestEndpointConfig(_ string, _ string, natsURL string) *config.EndpointConfig {
	return &config.EndpointConfig{
		NATS: config.NATSConfig{
			URL: natsURL,
		},
		Security: config.SecurityConfig{
			HMACSecret: "test-hmac-secret-for-integration-tests",
		},
		Observability: config.ObservabilityConfig{
			LogLevel:       "debug",
			LogFormat:      "json",
			MetricsEnabled: false,
		},
		ID:               "test-endpoint-1",
		Tags:             []string{"test", "integration"},
		ConcurrencyLimit: testConcurrencyLimit,
		MaxPoolHosts:     testMaxPoolHosts,
		IdleConnsPerHost: testIdleConnsPerHost,
		IdleConnTimeout:  testIdleConnTimeout,
	}
}

// CleanDatabase truncates integration test tables.
func CleanDatabase(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for cleanup: %w", err)
	}
	defer func() { _ = db.Close() }()

	tables := []string{
		"usage_daily_summary",
		"usage_records",
		"cost_multipliers",
		"admin_audit_log",
		"audit_log",
		"routing_rules",
		"api_keys",
		"endpoints",
		"fingerprint_presets",
	}

	for _, table := range tables {
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)

		_, err := db.ExecContext(ctx, query)
		if err != nil {
			continue
		}
	}

	return nil
}

// CleanRedis flushes all data from a Redis instance used in tests.
func CleanRedis(ctx context.Context, addr string) error {
	client, err := redis.NewClient(ctx, config.RedisConfig{Addr: addr}, nil)
	if err != nil {
		return fmt.Errorf("failed to open redis for cleanup: %w", err)
	}
	defer func() { _ = client.Close() }()

	err = client.Client.FlushDB(ctx).Err()
	if err != nil {
		return fmt.Errorf("flush redis: %w", err)
	}

	return nil
}

// WithTimeout creates a context with a timeout.
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// WaitForHealthy polls a health check function until it succeeds or the timeout expires.
func WaitForHealthy(ctx context.Context, healthCheck func() error, interval, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		err := healthCheck()
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		case <-time.After(interval):
		}
	}

	return fmt.Errorf("%w: %v", ErrHealthCheckTimeout, timeout)
}

// TestAPIKey holds an API key created for integration tests.
type TestAPIKey struct {
	ID        string
	Token     string
	RawKey    string
	TokenHash string
	Scopes    []string
	IsActive  bool
}

// CreateTestAPIKey inserts an API key into the test database.
func CreateTestAPIKey(ctx context.Context, dsn string, name string, scopes []string) (*TestAPIKey, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	id := uuid.New().String()

	token := uuid.New().String()

	tokenHash := sha256Hash(token)

	if len(scopes) == 1 && scopes[0] == "*" {
		scopes = []string{}
	}

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scopes: %w", err)
	}

	if scopes == nil {
		scopesJSON = []byte("[]")
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, token_hash, scopes, is_active)
		VALUES ($1, $2, $3, $4, true)
	`, id, name, tokenHash, scopesJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to insert api key: %w", err)
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO api_key_tokens (api_key_id, token_hash, status)
		VALUES ($1, $2, 'active')
	`, id, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	return &TestAPIKey{
		ID:        id,
		Token:     token,
		RawKey:    token,
		TokenHash: tokenHash,
		Scopes:    scopes,
		IsActive:  true,
	}, nil
}

// TestEndpointPool defines an endpoint pool for routing rules.
type TestEndpointPool struct {
	Tier       int
	Endpoints  []string
	MaxRetries int
}

// CreateTestRoutingRule inserts a routing rule into the test database.
func CreateTestRoutingRule(
	ctx context.Context,
	dsn string,
	name string,
	priority int,
	requiredTags []string,
	excludedTags []string,
	quotaKey string,
	rateLimitPerSecond int,
	rateLimitPerMinute int,
	fingerprintPreset string,
	endpointPools []TestEndpointPool,
) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	rule := newTestRoutingRule(
		name,
		priority,
		requiredTags,
		excludedTags,
		quotaKey,
		rateLimitPerSecond,
		rateLimitPerMinute,
		fingerprintPreset,
		endpointPools,
	)

	configJSON, reqTagsJSON, exclTagsJSON, err := testRoutingRuleJSON(rule)
	if err != nil {
		return err
	}

	return insertTestRoutingRule(ctx, db, rule, configJSON, reqTagsJSON, exclTagsJSON)
}

func newTestRoutingRule(
	name string,
	priority int,
	requiredTags []string,
	excludedTags []string,
	quotaKey string,
	rateLimitPerSecond int,
	rateLimitPerMinute int,
	fingerprintPreset string,
	endpointPools []TestEndpointPool,
) domain.RoutingRule {
	id := uuid.New().String()
	if name == "" {
		name = "Test Rule " + id
	}

	if priority == 0 {
		priority = 100
	}

	domainPools := make([]domain.EndpointPool, len(endpointPools))
	for i, p := range endpointPools {
		domainPools[i] = domain.EndpointPool{
			Tier:       p.Tier,
			Endpoints:  p.Endpoints,
			MaxRetries: p.MaxRetries,
		}
	}

	rule := domain.RoutingRule{
		ID:                 id,
		Name:               name,
		Priority:           priority,
		RequiredTags:       requiredTags,
		ExcludedTags:       excludedTags,
		QuotaKey:           quotaKey,
		RateLimitPerSecond: rateLimitPerSecond,
		RateLimitPerMinute: rateLimitPerMinute,
		FingerprintPreset:  fingerprintPreset,
		EndpointPools:      domainPools,
		IsActive:           true,
		Version:            1,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	return rule
}

func testRoutingRuleJSON(rule domain.RoutingRule) ([]byte, []byte, []byte, error) {
	configJSON, err := json.Marshal(rule)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to marshal rule config: %w", err)
	}

	reqTagsJSON, err := testStringSliceJSON(rule.RequiredTags, "required tags")
	if err != nil {
		return nil, nil, nil, err
	}

	exclTagsJSON, err := testStringSliceJSON(rule.ExcludedTags, "excluded tags")
	if err != nil {
		return nil, nil, nil, err
	}

	return configJSON, reqTagsJSON, exclTagsJSON, nil
}

func testStringSliceJSON(values []string, name string) ([]byte, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s: %w", name, err)
	}

	if values == nil {
		return []byte("[]"), nil
	}

	return data, nil
}

func insertTestRoutingRule(ctx context.Context, db *sql.DB, rule domain.RoutingRule, configJSON, reqTagsJSON, exclTagsJSON []byte) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO routing_rules (
			id, name, priority, required_tags, excluded_tags, config,
			is_active, version, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`, rule.ID, rule.Name, rule.Priority, reqTagsJSON, exclTagsJSON, configJSON,
		true, 1, time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to create routing rule: %w", err)
	}

	return nil
}

// TestEndpoint represents an endpoint for integration tests.
type TestEndpoint struct {
	ID        string
	Tags      []string
	IsHealthy bool
}

// CreateTestEndpoint inserts an endpoint into the test database.
func CreateTestEndpoint(ctx context.Context, dsn string, endpoint *TestEndpoint) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	if endpoint.ID == "" {
		endpoint.ID = fmt.Sprintf("endpoint_%d", time.Now().UnixNano())
	}

	tagsJSON, err := json.Marshal(endpoint.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	if endpoint.Tags == nil {
		tagsJSON = []byte("[]")
	}

	metadataJSON := []byte(`{"version": "test", "active_tasks": 0}`)

	_, err = db.ExecContext(ctx, `
		INSERT INTO endpoints (id, tags, last_heartbeat, is_healthy, metadata, created_at)
		VALUES ($1, $2, NOW(), $3, $4, NOW())
	`, endpoint.ID, tagsJSON, endpoint.IsHealthy, metadataJSON)
	if err != nil {
		return fmt.Errorf("failed to create endpoint: %w", err)
	}

	return nil
}

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))

	return hex.EncodeToString(hash[:])
}

// HTTPTestClient is an HTTP client for integration tests.
type HTTPTestClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

// NewHTTPTestClient creates an HTTP test client.
func NewHTTPTestClient(baseURL, apiKey string) *HTTPTestClient {
	return &HTTPTestClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: testHTTPTimeout},
	}
}

// ProxyRequest is a proxied HTTP request for integration tests.
type ProxyRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	Body      []byte
	SessionID string
	Tags      []string
}

// ProxyResponse is a proxied HTTP response from integration tests.
type ProxyResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// SendRequest sends a proxied HTTP request through the relay.
func (c *HTTPTestClient) SendRequest(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
	if req.Method == "" {
		req.Method = httpGet
	}

	httpReq, err := c.newProxyHTTPRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	return readProxyResponse(resp)
}

func (c *HTTPTestClient) newProxyHTTPRequest(ctx context.Context, req *ProxyRequest) (*http.Request, error) {
	body, err := json.Marshal(proxyPayload(req))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/request", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	applyProxyRequestHeaders(httpReq, c.APIKey, req)

	return httpReq, nil
}

func proxyPayload(req *ProxyRequest) map[string]any {
	payload := map[string]any{
		"url":    req.URL,
		"method": req.Method,
	}
	if len(req.Headers) > 0 {
		headers := make([]map[string]string, 0, len(req.Headers))
		for k, v := range req.Headers {
			headers = append(headers, map[string]string{"key": k, "value": v})
		}

		payload["headers"] = headers
	}

	if len(req.Body) > 0 {
		payload["body"] = req.Body
	}

	if req.SessionID != "" {
		payload["session_id"] = req.SessionID
	}

	return payload
}

func applyProxyRequestHeaders(httpReq *http.Request, apiKey string, req *ProxyRequest) {
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	if len(req.Tags) > 0 {
		httpReq.Header.Set("X-Relay-Tags", strings.Join(req.Tags, ","))
	}

	if req.SessionID != "" {
		httpReq.Header.Set("X-Session-ID", req.SessionID)
	}
}

func readProxyResponse(resp *http.Response) (*ProxyResponse, error) {
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &ProxyResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

// NewTestRedisClient creates a Redis client for integration tests.
func NewTestRedisClient(ctx context.Context, t testing.TB, addr string) *redis.Client {
	cfg := config.RedisConfig{
		Addr: addr,
	}
	client, err := redis.NewClient(ctx, cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = client.Close()
	})

	return client
}

// NewTestAuthRepo creates an APIKeyRepository for integration tests.
func NewTestAuthRepo(t testing.TB, dsn string) domain.APIKeyRepository {
	client, err := postgres.NewClient(context.Background(), dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})

	return postgres.NewAPIKeyRepository(client)
}

// NewTestAuthTokenRepo creates an APIKeyTokenRepository for integration tests.
func NewTestAuthTokenRepo(t testing.TB, dsn string) domain.APIKeyTokenRepository {
	client, err := postgres.NewClient(context.Background(), dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})

	return postgres.NewAPIKeyTokenRepository(client)
}

// NewTestRuleRepo creates a RoutingRuleRepository for integration tests.
func NewTestRuleRepo(t testing.TB, dsn string) domain.RoutingRuleRepository {
	client, err := postgres.NewClient(context.Background(), dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})

	return postgres.NewRoutingRuleRepository(client)
}

// ExecuteSQL runs a SQL query in integration tests.
func ExecuteSQL(ctx context.Context, t testing.TB, dsn string, query string, args ...any) {
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)

	defer func() { _ = db.Close() }()

	_, err = db.ExecContext(ctx, query, args...)
	require.NoError(t, err)
}

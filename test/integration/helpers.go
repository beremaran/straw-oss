package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/kwilabs/straw-proxy-server/internal/config"
	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/postgres"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// NewTestServerConfig creates a ServerConfig with container connection strings.
func NewTestServerConfig(postgresDSN, redisAddr, rabbitmqURL string) *config.ServerConfig {
	return &config.ServerConfig{
		Core: config.CoreConfig{
			PostgresDSN: postgresDSN,
			RedisAddr:   redisAddr,
			RabbitMQURL: rabbitmqURL,
			LogLevel:    "debug",
			LogFormat:   "json",
		},
		Security: config.SecurityConfig{
			HMACSecret: "test-hmac-secret-for-integration-tests",
		},
		Observability: config.ObservabilityConfig{
			MetricsEnabled: false,
		},
		HTTPPort:        0, // Use any available port
		AdminPort:       0,
		ShutdownTimeout: 5 * time.Second,
		ResultTimeout:   30 * time.Second,
		AdminAPIKey:     "test-admin-api-key",
		MaxBodySize:     "2M",
	}
}

// NewTestEndpointConfig creates an EndpointConfig with container connection strings.
func NewTestEndpointConfig(postgresDSN, redisAddr, rabbitmqURL string) *config.EndpointConfig {
	return &config.EndpointConfig{
		Core: config.CoreConfig{
			PostgresDSN: postgresDSN,
			RedisAddr:   redisAddr,
			RabbitMQURL: rabbitmqURL,
			LogLevel:    "debug",
			LogFormat:   "json",
		},
		Security: config.SecurityConfig{
			HMACSecret: "test-hmac-secret-for-integration-tests",
		},
		Observability: config.ObservabilityConfig{
			MetricsEnabled: false,
		},
		ID:               "test-endpoint-1",
		Tags:             []string{"test", "integration"},
		ConcurrencyLimit: 100,
		MaxPoolHosts:     10,
		IdleConnsPerHost: 2,
		IdleConnTimeout:  90 * time.Second,
	}
}

// CleanDatabase truncates all application tables to ensure test isolation.
// This is useful for per-test cleanup when sharing containers across tests.
func CleanDatabase(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for cleanup: %w", err)
	}
	defer db.Close()

	// Truncate tables in order respecting foreign key constraints
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
		// Use CASCADE to handle any foreign key dependencies
		query := fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)
		if _, err := db.ExecContext(ctx, query); err != nil {
			// Table might not exist if migrations haven't created it yet
			// or if schema has changed - log and continue
			continue
		}
	}

	return nil
}

// WithTimeout returns a context with the specified timeout.
// Useful for wrapping test operations with a max execution time.
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// WaitForHealthy polls a health check function until it succeeds or times out.
func WaitForHealthy(ctx context.Context, healthCheck func() error, interval, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := healthCheck(); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			// Continue polling
		}
	}

	return fmt.Errorf("health check timed out after %v", timeout)
}

// TestAPIKey holds test API key data including the raw secret.
type TestAPIKey struct {
	ID       string
	Secret   string
	RawKey   string // Format: "id:secret"
	KeyHash  string
	Scopes   []string
	IsActive bool
}

// CreateTestAPIKey creates an API key in the database and returns the credentials.
func CreateTestAPIKey(ctx context.Context, dsn string, name string, scopes []string) (*TestAPIKey, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	id := uuid.New().String()
	secret := fmt.Sprintf("secret_%d", time.Now().UnixNano())
	rawKey := id + ":" + secret

	// Hash the secret with bcrypt
	hash, err := hashPassword(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Handle wildcard "*" by converting to empty array for no scope restrictions
	// The validation logic expects tags in key:value or key=value format, not "*"
	if len(scopes) == 1 && scopes[0] == "*" {
		scopes = []string{}
	}
	scopesJSON, _ := json.Marshal(scopes)
	if scopes == nil {
		scopesJSON = []byte("[]")
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, key_hash, scopes, is_active)
		VALUES ($1, $2, $3, $4, true)
	`, id, name, hash, scopesJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	return &TestAPIKey{
		ID:       id,
		Secret:   secret,
		RawKey:   rawKey,
		KeyHash:  hash,
		Scopes:   scopes,
		IsActive: true,
	}, nil
}

// TestEndpointPool defines an endpoint pool for testing.
type TestEndpointPool struct {
	Tier       int
	Endpoints  []string
	MaxRetries int
}

// CreateTestRoutingRule creates a routing rule in the database.
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
	defer db.Close()

	id := uuid.New().String()
	if name == "" {
		name = "Test Rule " + id
	}
	if priority == 0 {
		priority = 100
	}

	// Convert TestEndpointPool to domain.EndpointPool
	domainPools := make([]domain.EndpointPool, len(endpointPools))
	for i, p := range endpointPools {
		domainPools[i] = domain.EndpointPool{
			Tier:       p.Tier,
			Endpoints:  p.Endpoints,
			MaxRetries: p.MaxRetries,
		}
	}

	// Create the domain.RoutingRule struct
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

	// Marshal the full rule to config
	configJSON, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("failed to marshal rule config: %w", err)
	}

	// Marshal tags
	reqTagsJSON, err := json.Marshal(requiredTags)
	if err != nil {
		return fmt.Errorf("failed to marshal required tags: %w", err)
	}
	if requiredTags == nil {
		reqTagsJSON = []byte("[]")
	}

	exclTagsJSON, err := json.Marshal(excludedTags)
	if err != nil {
		return fmt.Errorf("failed to marshal excluded tags: %w", err)
	}
	if excludedTags == nil {
		exclTagsJSON = []byte("[]")
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO routing_rules (
			id, name, priority, required_tags, excluded_tags, config,
			is_active, version, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`, id, name, priority, reqTagsJSON, exclTagsJSON, configJSON,
		true, 1, time.Now(), time.Now())
	if err != nil {
		return fmt.Errorf("failed to create routing rule: %w", err)
	}

	return nil
}

// TestEndpoint holds test endpoint data.
type TestEndpoint struct {
	ID        string
	Tags      []string
	IsHealthy bool
}

// CreateTestEndpoint creates an endpoint record in the database.
func CreateTestEndpoint(ctx context.Context, dsn string, endpoint *TestEndpoint) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if endpoint.ID == "" {
		endpoint.ID = fmt.Sprintf("endpoint_%d", time.Now().UnixNano())
	}

	tagsJSON, _ := json.Marshal(endpoint.Tags)
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

// hashPassword hashes a password using bcrypt.
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// HTTPTestClient is a helper for making authenticated HTTP requests.
type HTTPTestClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

// NewHTTPTestClient creates a new HTTP test client.
func NewHTTPTestClient(baseURL, apiKey string) *HTTPTestClient {
	return &HTTPTestClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// ProxyRequest represents a request to send through the proxy.
type ProxyRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	Body      []byte
	SessionID string
	Tags      []string
}

// ProxyResponse represents the response from the proxy.
type ProxyResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// SendRequest sends a request through the proxy.
func (c *HTTPTestClient) SendRequest(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
	if req.Method == "" {
		req.Method = "GET"
	}

	// Build request body
	payload := map[string]interface{}{
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

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/request", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.APIKey)
	if len(req.Tags) > 0 {
		httpReq.Header.Set("X-Relay-Tags", strings.Join(req.Tags, ","))
	}
	if req.SessionID != "" {
		httpReq.Header.Set("X-Session-ID", req.SessionID)
	}

	// Send request
	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body
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

// NewTestRedisClient creates a new Redis client for testing.
func NewTestRedisClient(t testing.TB, addr string) *redis.Client {
	client, err := redis.NewClient(addr, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})
	return client
}

// NewTestAuthRepo creates a new ApiKeyRepository for testing.
func NewTestAuthRepo(t testing.TB, dsn string) domain.ApiKeyRepository {
	client, err := postgres.NewClient(context.Background(), dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})
	return postgres.NewApiKeyRepository(client)
}

// NewTestRuleRepo creates a new RoutingRuleRepository for testing.
func NewTestRuleRepo(t testing.TB, dsn string) domain.RoutingRuleRepository {
	client, err := postgres.NewClient(context.Background(), dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})
	return postgres.NewRoutingRuleRepository(client)
}

// ExecuteSQL executes a raw SQL statement.
func ExecuteSQL(t testing.TB, dsn string, query string, args ...interface{}) {
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(query, args...)
	require.NoError(t, err)
}

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/config"
	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/postgres"
	"github.com/beremaran/straw/internal/infra/redis"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
)

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
		HTTPPort:        0,
		AdminPort:       0,
		ShutdownTimeout: 5 * time.Second,
		ResultTimeout:   30 * time.Second,
		AdminAPIKey:     "test-admin-api-key",
		MaxBodySize:     "2M",
	}
}

func NewTestEndpointConfig(postgresDSN, redisAddr, natsURL string) *config.EndpointConfig {
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
		ConcurrencyLimit: 100,
		MaxPoolHosts:     10,
		IdleConnsPerHost: 2,
		IdleConnTimeout:  90 * time.Second,
	}
}

func CleanDatabase(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database for cleanup: %w", err)
	}
	defer db.Close()

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
		if _, err := db.ExecContext(ctx, query); err != nil {

			continue
		}
	}

	return nil
}

func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

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

		}
	}

	return fmt.Errorf("health check timed out after %v", timeout)
}

type TestAPIKey struct {
	ID        string
	Token     string
	RawKey    string
	TokenHash string
	Scopes    []string
	IsActive  bool
}

func CreateTestAPIKey(ctx context.Context, dsn string, name string, scopes []string) (*TestAPIKey, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	id := uuid.New().String()

	token := uuid.New().String()

	tokenHash := sha256Hash(token)

	if len(scopes) == 1 && scopes[0] == "*" {
		scopes = []string{}
	}
	scopesJSON, _ := json.Marshal(scopes)
	if scopes == nil {
		scopesJSON = []byte("[]")
	}

	_, err = db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, token_hash, scopes, is_active)
		VALUES ($1, $2, $3, $4, true)
	`, id, name, tokenHash, scopesJSON)
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

type TestEndpointPool struct {
	Tier       int
	Endpoints  []string
	MaxRetries int
}

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

	configJSON, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("failed to marshal rule config: %w", err)
	}

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

type TestEndpoint struct {
	ID        string
	Tags      []string
	IsHealthy bool
}

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

func sha256Hash(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}

type HTTPTestClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

func NewHTTPTestClient(baseURL, apiKey string) *HTTPTestClient {
	return &HTTPTestClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{Timeout: 30 * time.Second},
	}
}

type ProxyRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	Body      []byte
	SessionID string
	Tags      []string
}

type ProxyResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func (c *HTTPTestClient) SendRequest(ctx context.Context, req *ProxyRequest) (*ProxyResponse, error) {
	if req.Method == "" {
		req.Method = "GET"
	}

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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/request", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	if len(req.Tags) > 0 {
		httpReq.Header.Set("X-Relay-Tags", strings.Join(req.Tags, ","))
	}
	if req.SessionID != "" {
		httpReq.Header.Set("X-Session-ID", req.SessionID)
	}

	resp, err := c.Client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

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

func NewTestRedisClient(t testing.TB, addr string) *redis.Client {
	cfg := config.RedisConfig{
		Addr: addr,
	}
	client, err := redis.NewClient(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})
	return client
}

func NewTestAuthRepo(t testing.TB, dsn string) domain.ApiKeyRepository {
	client, err := postgres.NewClient(context.Background(), dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})
	return postgres.NewApiKeyRepository(client)
}

func NewTestRuleRepo(t testing.TB, dsn string) domain.RoutingRuleRepository {
	client, err := postgres.NewClient(context.Background(), dsn, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		client.Close()
	})
	return postgres.NewRoutingRuleRepository(client)
}

func ExecuteSQL(t testing.TB, dsn string, query string, args ...interface{}) {
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer db.Close()

	_, err = db.Exec(query, args...)
	require.NoError(t, err)
}

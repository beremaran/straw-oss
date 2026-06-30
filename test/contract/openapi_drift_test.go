package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"

	"github.com/beremaran/straw/internal/server/dto"
)

const (
	regionUS        = "region:us"
	startDate       = "2026-06-01"
	endDate         = "2026-06-28"
	targetAll       = "target:*"
	chromePreset    = "chrome-130"
	primaryUUID     = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	secondaryUUID   = "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12"
	typeResidential = "type:residential"
)

// loadOpenAPIDoc loads the api/openapi.yaml file and parses it as a map.
func loadOpenAPIDoc(t *testing.T) map[string]any {
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Traverse up to find api/openapi.yaml
	path := filepath.Join(wd, "api", "openapi.yaml")
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		path = filepath.Join(wd, "..", "api", "openapi.yaml")
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		path = filepath.Join(wd, "..", "..", "api", "openapi.yaml")
	}

	yamlBytes, err := os.ReadFile(filepath.Clean(path))
	require.NoError(t, err, "failed to read openapi.yaml")

	var doc map[string]any
	err = yaml.Unmarshal(yamlBytes, &doc)
	require.NoError(t, err, "failed to parse openapi.yaml as YAML")

	// Convert YAML map (which has map[interface{}]interface{} or similar in older parsers,
	// but yaml.v3 uses map[string]interface{}) to JSON-compatible types.
	jsonBytes, err := json.Marshal(doc)
	require.NoError(t, err)

	var jsonDoc map[string]any
	err = json.Unmarshal(jsonBytes, &jsonDoc)
	require.NoError(t, err)

	return jsonDoc
}

// normalizeOpenAPISchema converts OpenAPI schema features (like nullable: true) to standard draft-04 JSON Schema.
func normalizeOpenAPISchema(node any) any {
	m, ok := node.(map[string]any)
	if !ok {
		a, ok := node.([]any)
		if ok {
			for i, v := range a {
				a[i] = normalizeOpenAPISchema(v)
			}

			return a
		}

		return node
	}

	for k, v := range m {
		m[k] = normalizeOpenAPISchema(v)
	}

	handleNullable(m)

	return m
}

func handleNullable(m map[string]any) {
	nullable, ok := m["nullable"].(bool)
	if !ok || !nullable {
		return
	}
	delete(m, "nullable")

	t, ok := m["type"].(string)
	if ok {
		m["type"] = []any{t, "null"}

		return
	}

	types, ok := m["type"].([]any)
	if !ok {
		return
	}

	hasNull := false
	for _, typ := range types {
		if typ == "null" {
			hasNull = true

			break
		}
	}
	if !hasNull {
		m["type"] = append(types, "null")
	}
}

func validateAgainstOpenAPISchema(t *testing.T, doc map[string]any, schemaName string, data any) *gojsonschema.Result {
	components := normalizeOpenAPISchema(doc["components"])

	wrapper := map[string]any{
		"$schema":    "http://json-schema.org/draft-04/schema#",
		"$ref":       "#/components/schemas/" + schemaName,
		"components": components,
	}

	schemaLoader := gojsonschema.NewGoLoader(wrapper)
	schema, err := gojsonschema.NewSchema(schemaLoader)
	require.NoError(t, err, "failed to compile OpenAPI schema for %s", schemaName)

	jsonBytes, err := json.Marshal(data)
	require.NoError(t, err, "failed to marshal DTO to JSON")

	loader := gojsonschema.NewBytesLoader(jsonBytes)
	result, err := schema.Validate(loader)
	require.NoError(t, err, "failed to validate schema for %s", schemaName)

	return result
}

func assertValid(t *testing.T, doc map[string]any, schemaName string, data any) {
	result := validateAgainstOpenAPISchema(t, doc, schemaName, data)
	if !result.Valid() {
		t.Errorf("Schema validation failed for %s. Errors:", schemaName)
		for _, desc := range result.Errors() {
			t.Errorf("- %s", desc)
		}
		t.FailNow()
	}
}

func assertInvalid(t *testing.T, doc map[string]any, schemaName string, data any) {
	result := validateAgainstOpenAPISchema(t, doc, schemaName, data)
	assert.False(t, result.Valid(), "expected schema validation to fail for %s, but it passed", schemaName)
}

func TestOpenApiSpecificationDrift(t *testing.T) {
	doc := loadOpenAPIDoc(t)

	t.Run("RelayRequest Schema", func(t *testing.T) {
		validReq := dto.RelayRequest{
			ID:              "req-1",
			Method:          "GET",
			URL:             "https://example.com/target",
			Headers:         map[string]string{"User-Agent": "straw-proxy"},
			Body:            []byte("hello-body"),
			Timeout:         "30s",
			SessionID:       "sess-123",
			TraceID:         "trace-456",
			StreamResponse:  false,
			MaxResponseSize: 1024 * 1024,
		}
		assertValid(t, doc, "RelayRequest", validReq)

		// Invalid: missing required "url" field
		invalidReq := map[string]any{
			"id":     "req-2",
			"method": "POST",
		}
		assertInvalid(t, doc, "RelayRequest", invalidReq)
	})

	t.Run("CreateAPIKeyRequest Schema", func(t *testing.T) {
		validReq := dto.CreateAPIKeyRequest{
			Name:              "Prod Scraper Key",
			Scopes:            []string{targetAll, regionUS},
			RateLimitOverride: func(v int) *int { return &v }(100),
		}
		assertValid(t, doc, "CreateAPIKeyRequest", validReq)

		// Invalid: missing required "name" field
		invalidReq := map[string]any{
			"scopes": []string{targetAll},
		}
		assertInvalid(t, doc, "CreateAPIKeyRequest", invalidReq)
	})

	t.Run("APIKeyResponse Schema", func(t *testing.T) {
		now := time.Now()
		validRes := dto.APIKeyResponse{
			ID:                primaryUUID,
			Name:              "Test Key",
			Scopes:            []string{targetAll},
			RateLimitOverride: nil,
			IsActive:          true,
			CreatedAt:         now,
			ExpiresAt:         &now,
		}
		assertValid(t, doc, "APIKeyResponse", validRes)
	})

	t.Run("CreateAPIKeyResponse Schema", func(t *testing.T) {
		now := time.Now()
		validRes := dto.CreateAPIKeyResponse{
			APIKeyResponse: dto.APIKeyResponse{
				ID:        primaryUUID,
				Name:      "Test Key",
				Scopes:    []string{targetAll},
				IsActive:  true,
				CreatedAt: now,
			},
			RawKey: "4cf5dfa6b4b4b4b4b4",
		}
		assertValid(t, doc, "CreateAPIKeyResponse", validRes)
	})

	t.Run("CreateRoutingRuleRequest Schema", func(t *testing.T) {
		validRule := dto.CreateRoutingRuleRequest{
			Name:                 "US Residential Egress",
			RequiredTags:         []string{typeResidential, regionUS},
			ExcludedTags:         []string{"type:datacenter"},
			Priority:             100,
			HardTimeout:          "30s",
			RateLimitPerMinute:   500,
			RateLimitPerSecond:   10,
			AllowedEndpointTypes: []string{"residential"},
			RequiredEndpointCaps: []string{"http2"},
			FingerprintPreset:    chromePreset,
			FingerprintABTest: &dto.ABConfigDTO{
				Variants: []dto.ABVariantDTO{
					{Fingerprint: chromePreset, Weight: 50},
					{Fingerprint: "firefox-128", Weight: 50},
				},
				Strategy: "random",
			},
			QuotaKey:         "us-quota",
			AllowInsecureTLS: false,
			PinnedCertHash:   "sha256/hash",
			RequestFilters: &dto.RequestFilterDTO{
				BlockContentTypes: []string{"image/png"},
				BlockURLPatterns:  []string{"*.doubleclick.net/*"},
				BlockDomains:      []string{"ads.com"},
				EnableAdblock:     true,
				AdblockLists:      []string{"easylist"},
			},
			EndpointPools: []dto.EndpointPoolDTO{
				{Tier: 1, Endpoints: []string{"ep-1"}, MaxRetries: 3},
			},
			IsActive: true,
		}
		assertValid(t, doc, "CreateRoutingRuleRequest", validRule)

		// Invalid: missing required "name"
		invalidRule := map[string]any{
			"priority": 100,
		}
		assertInvalid(t, doc, "CreateRoutingRuleRequest", invalidRule)
	})

	t.Run("RoutingRuleResponse Schema", func(t *testing.T) {
		validRule := dto.RoutingRuleResponse{
			ID:                   secondaryUUID,
			Name:                 "Rule 1",
			Priority:             10,
			IsActive:             true,
			Version:              1,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
			AllowedEndpointTypes: []string{},
			RequiredEndpointCaps: []string{},
			RequiredTags:         []string{},
			ExcludedTags:         []string{},
			EndpointPools:        []dto.EndpointPoolDTO{},
		}
		assertValid(t, doc, "RoutingRuleResponse", validRule)
	})

	t.Run("EndpointHealthResponse Schema", func(t *testing.T) {
		validEp := dto.EndpointHealthResponse{
			EndpointID:  "worker-1",
			State:       "healthy",
			Tags:        []string{regionUS},
			Version:     "1.0.0",
			ActiveTasks: 5,
			LastSeen:    time.Now().Format(time.RFC3339),
		}
		assertValid(t, doc, "EndpointHealthResponse", validEp)
	})

	t.Run("FingerprintResponse Schema", func(t *testing.T) {
		validFp := dto.FingerprintResponse{
			ID:   "fp-1",
			Name: "Chrome Desktop",
			Config: map[string]any{
				"user_agent": "Mozilla/5.0",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		assertValid(t, doc, "FingerprintResponse", validFp)
	})

	t.Run("UsageSummaryResponse Schema", func(t *testing.T) {
		validUsage := dto.UsageSummaryResponse{
			Data: []dto.UsageSummaryDTO{
				{
					Date:          endDate,
					TotalRequests: 1500,
					TotalBytes:    5000000,
					CostUnits:     1.5,
					Breakdown:     map[string]int64{"residential": 1500},
				},
			},
			Start: startDate,
			End:   endDate,
		}
		assertValid(t, doc, "UsageSummaryResponse", validUsage)
	})

	t.Run("BillingEstimateResponse Schema", func(t *testing.T) {
		validBilling := dto.BillingEstimateResponse{
			TotalCostUnits: 150.0,
			EstimatedUSD:   1.50,
			Currency:       "USD",
			Start:          startDate,
			End:            endDate,
			PricingVersion: "base",
			Multipliers: []dto.BillingMultiplierDTO{
				{EndpointTag: typeResidential, Multiplier: 1.5, Version: 1},
			},
		}
		assertValid(t, doc, "BillingEstimateResponse", validBilling)
	})

	t.Run("CostMultiplierResponse Schema", func(t *testing.T) {
		validMultiplier := dto.CostMultiplierResponse{
			ID:          primaryUUID,
			EndpointTag: typeResidential,
			Multiplier:  1.5,
			Description: "Residential traffic",
			IsActive:    true,
			Version:     1,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		assertValid(t, doc, "CostMultiplierResponse", validMultiplier)
	})

	t.Run("ReportResponse Schema", func(t *testing.T) {
		validReport := dto.ReportResponse{
			ID:        primaryUUID,
			Name:      "Usage",
			Type:      "usage_summary",
			Filters:   map[string]any{"start": startDate, "end": endDate},
			Format:    "csv",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		assertValid(t, doc, "ReportResponse", validReport)
	})

	t.Run("NotificationChannelResponse Schema", func(t *testing.T) {
		validChannel := dto.NotificationChannelResponse{
			ID:        primaryUUID,
			Name:      "Ops",
			Type:      "webhook",
			Config:    map[string]any{"team": "ops"},
			HasSecret: true,
			IsEnabled: true,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		assertValid(t, doc, "NotificationChannelResponse", validChannel)
	})

	t.Run("AlertRuleResponse Schema", func(t *testing.T) {
		validRule := dto.AlertRuleResponse{
			ID:                     primaryUUID,
			Name:                   "High requests",
			Metric:                 "usage_requests",
			Condition:              "greater_than",
			Threshold:              100,
			Window:                 "5m",
			Filters:                map[string]any{},
			Severity:               "warning",
			IsActive:               true,
			Cooldown:               "15m",
			NotificationChannelIDs: []string{secondaryUUID},
			CreatedAt:              time.Now(),
			UpdatedAt:              time.Now(),
		}
		assertValid(t, doc, "AlertRuleResponse", validRule)
	})

	t.Run("AlertEventResponse Schema", func(t *testing.T) {
		resolvedAt := time.Now()
		validEvent := dto.AlertEventResponse{
			ID:          primaryUUID,
			AlertRuleID: secondaryUUID,
			Status:      "resolved",
			Value:       0,
			StartedAt:   time.Now(),
			ResolvedAt:  &resolvedAt,
			Details:     map[string]any{"metric": "usage_requests"},
		}
		assertValid(t, doc, "AlertEventResponse", validEvent)
	})

	t.Run("ClearCacheResponse Schema", func(t *testing.T) {
		validClear := dto.ClearCacheResponse{
			Message: "cache cleared",
			Pattern: "*",
			Deleted: 10,
		}
		assertValid(t, doc, "ClearCacheResponse", validClear)
	})
}

func TestOpenAPIIncludesManagementRoutes(t *testing.T) {
	doc := loadOpenAPIDoc(t)
	paths, ok := doc["paths"].(map[string]any)
	require.True(t, ok)

	for _, path := range []string{
		"/management/auth/login",
		"/management/auth/refresh",
		"/management/auth/logout",
		"/management/auth/me",
		"/management/users/bootstrap",
		"/management/auth/sso/{provider}/start",
		"/management/auth/sso/{provider}/callback",
		"/management/api-keys",
		"/management/api-keys/{id}",
		"/management/api-keys/{id}/rotate",
		"/management/api-keys/{id}/reactivate",
		"/management/api-keys/{id}/revoke",
		"/management/rules",
		"/management/rules/{id}",
		"/management/endpoints",
		"/management/endpoints/{id}",
		"/management/endpoints/{id}/drain",
		"/management/endpoints/{id}/undrain",
		"/management/endpoints/{id}/restart",
		"/management/endpoints/{id}/commands",
		"/management/commands/{id}",
		"/management/endpoints/{id}/logs",
		"/management/endpoints/{id}/logs/stream",
		"/management/fingerprints",
		"/management/fingerprints/{id}",
		"/management/fingerprints/broadcast",
		"/management/usage/summary",
		"/management/billing/estimate",
		"/management/cost-multipliers",
		"/management/cost-multipliers/{id}",
		"/management/reports",
		"/management/reports/{id}",
		"/management/reports/{id}/run",
		"/management/reports/{id}/runs",
		"/management/report-runs/{run_id}",
		"/management/report-runs/{run_id}/download",
		"/management/report-schedules",
		"/management/report-schedules/{id}",
		"/management/notification-channels",
		"/management/notification-channels/{id}",
		"/management/notification-channels/{id}/test",
		"/management/notification-preferences",
		"/management/alerts/rules",
		"/management/alerts/rules/{id}",
		"/management/alerts/events",
		"/management/alerts/events/{id}/ack",
		"/management/cache/clear",
		"/management/cache/stats",
		"/management/audit/events",
		"/management/audit/events/{id}",
		"/management/audit/requests",
		"/management/audit/export",
		"/management/users",
		"/management/users/{id}",
		"/management/roles",
		"/management/roles/{id}",
		"/management/identity-providers",
		"/management/identity-providers/{id}",
	} {
		assert.Contains(t, paths, path)
	}
}

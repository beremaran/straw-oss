package contract_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/server/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v3"
)

// loadOpenApiDoc loads the api/openapi.yaml file and parses it as a map.
func loadOpenApiDoc(t *testing.T) map[string]interface{} {
	wd, err := os.Getwd()
	require.NoError(t, err)

	// Traverse up to find api/openapi.yaml
	path := filepath.Join(wd, "api", "openapi.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(wd, "..", "api", "openapi.yaml")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join(wd, "..", "..", "api", "openapi.yaml")
	}

	absPath, err := filepath.Abs(path)
	require.NoError(t, err, "failed to resolve absolute path for openapi.yaml")

	yamlBytes, err := os.ReadFile(absPath)
	require.NoError(t, err, "failed to read openapi.yaml")

	var doc map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &doc)
	require.NoError(t, err, "failed to parse openapi.yaml as YAML")

	// Convert YAML map (which has map[interface{}]interface{} or similar in older parsers,
	// but yaml.v3 uses map[string]interface{}) to JSON-compatible types.
	jsonBytes, err := json.Marshal(doc)
	require.NoError(t, err)

	var jsonDoc map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonDoc)
	require.NoError(t, err)

	return jsonDoc
}

// normalizeOpenApiSchema converts OpenAPI schema features (like nullable: true) to standard draft-04 JSON Schema.
func normalizeOpenApiSchema(node interface{}) interface{} {
	m, ok := node.(map[string]interface{})
	if !ok {
		a, ok := node.([]interface{})
		if ok {
			for i, v := range a {
				a[i] = normalizeOpenApiSchema(v)
			}

			return a
		}

		return node
	}

	for k, v := range m {
		m[k] = normalizeOpenApiSchema(v)
	}

	// Convert "nullable: true" to "type: [type, null]"
	if nullable, ok := m["nullable"].(bool); ok && nullable {
		delete(m, "nullable")
		if t, ok := m["type"].(string); ok {
			m["type"] = []interface{}{t, "null"}
		} else if types, ok := m["type"].([]interface{}); ok {
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
	}

	return m
}

func validateAgainstOpenApiSchema(t *testing.T, doc map[string]interface{}, schemaName string, data interface{}) *gojsonschema.Result {
	components := normalizeOpenApiSchema(doc["components"])

	wrapper := map[string]interface{}{
		"$schema":    "http://json-schema.org/draft-04/schema#",
		"$ref":       fmt.Sprintf("#/components/schemas/%s", schemaName),
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

func assertValid(t *testing.T, doc map[string]interface{}, schemaName string, data interface{}) {
	result := validateAgainstOpenApiSchema(t, doc, schemaName, data)
	if !result.Valid() {
		t.Errorf("Schema validation failed for %s. Errors:", schemaName)
		for _, desc := range result.Errors() {
			t.Errorf("- %s", desc)
		}
		t.FailNow()
	}
}

func assertInvalid(t *testing.T, doc map[string]interface{}, schemaName string, data interface{}) {
	result := validateAgainstOpenApiSchema(t, doc, schemaName, data)
	assert.False(t, result.Valid(), "expected schema validation to fail for %s, but it passed", schemaName)
}

func TestOpenApiSpecificationDrift(t *testing.T) {
	doc := loadOpenApiDoc(t)

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
		invalidReq := map[string]interface{}{
			"id":     "req-2",
			"method": "POST",
		}
		assertInvalid(t, doc, "RelayRequest", invalidReq)
	})

	t.Run("CreateApiKeyRequest Schema", func(t *testing.T) {
		validReq := dto.CreateApiKeyRequest{
			Name:              "Prod Scraper Key",
			Scopes:            []string{"target:*", "region:us"},
			RateLimitOverride: func(v int) *int { return &v }(100),
		}
		assertValid(t, doc, "CreateApiKeyRequest", validReq)

		// Invalid: missing required "name" field
		invalidReq := map[string]interface{}{
			"scopes": []string{"target:*"},
		}
		assertInvalid(t, doc, "CreateApiKeyRequest", invalidReq)
	})

	t.Run("ApiKeyResponse Schema", func(t *testing.T) {
		now := time.Now()
		validRes := dto.ApiKeyResponse{
			ID:                "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			Name:              "Test Key",
			Scopes:            []string{"target:*"},
			RateLimitOverride: nil,
			IsActive:          true,
			CreatedAt:         now,
			ExpiresAt:         &now,
		}
		assertValid(t, doc, "ApiKeyResponse", validRes)
	})

	t.Run("CreateApiKeyResponse Schema", func(t *testing.T) {
		now := time.Now()
		validRes := dto.CreateApiKeyResponse{
			ApiKeyResponse: dto.ApiKeyResponse{
				ID:        "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
				Name:      "Test Key",
				Scopes:    []string{"target:*"},
				IsActive:  true,
				CreatedAt: now,
			},
			RawKey: "4cf5dfa6b4b4b4b4b4",
		}
		assertValid(t, doc, "CreateApiKeyResponse", validRes)
	})

	t.Run("CreateRoutingRuleRequest Schema", func(t *testing.T) {
		validRule := dto.CreateRoutingRuleRequest{
			Name:                 "US Residential Egress",
			RequiredTags:         []string{"type:residential", "region:us"},
			ExcludedTags:         []string{"type:datacenter"},
			Priority:             100,
			HardTimeout:          "30s",
			RateLimitPerMinute:   500,
			RateLimitPerSecond:   10,
			AllowedEndpointTypes: []string{"residential"},
			RequiredEndpointCaps: []string{"http2"},
			FingerprintPreset:    "chrome-130",
			FingerprintABTest: &dto.ABConfigDTO{
				Variants: []dto.ABVariantDTO{
					{Fingerprint: "chrome-130", Weight: 50},
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
		invalidRule := map[string]interface{}{
			"priority": 100,
		}
		assertInvalid(t, doc, "CreateRoutingRuleRequest", invalidRule)
	})

	t.Run("RoutingRuleResponse Schema", func(t *testing.T) {
		validRule := dto.RoutingRuleResponse{
			ID:                   "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12",
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
			Tags:        []string{"region:us"},
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
			Config: map[string]interface{}{
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
					Date:          "2026-06-28",
					TotalRequests: 1500,
					TotalBytes:    5000000,
					CostUnits:     1.5,
					Breakdown:     map[string]int64{"residential": 1500},
				},
			},
			Start: "2026-06-01",
			End:   "2026-06-28",
		}
		assertValid(t, doc, "UsageSummaryResponse", validUsage)
	})

	t.Run("BillingEstimateResponse Schema", func(t *testing.T) {
		validBilling := dto.BillingEstimateResponse{
			TotalCostUnits: 150.0,
			EstimatedUSD:   1.50,
			Currency:       "USD",
			Start:          "2026-06-01",
			End:            "2026-06-28",
		}
		assertValid(t, doc, "BillingEstimateResponse", validBilling)
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

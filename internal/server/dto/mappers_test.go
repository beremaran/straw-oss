package dto

import (
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayRequest_ToProtocolRequest(t *testing.T) {
	t.Run("basic conversion", func(t *testing.T) {
		dto := &RelayRequest{
			ID:        "test-id",
			Method:    "POST",
			URL:       "https://example.com/api",
			Headers:   map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
			Body:      []byte(`{"key": "value"}`),
			Timeout:   "30s",
			SessionID: "session-123",
			TraceID:   "trace-456",
		}

		result, err := dto.ToProtocolRequest()
		require.NoError(t, err)

		assert.Equal(t, "test-id", result.ID)
		assert.Equal(t, "POST", result.Method)
		assert.Equal(t, "https://example.com/api", result.URL)
		assert.Equal(t, 30*time.Second, result.Timeout)
		assert.Equal(t, "session-123", result.SessionID)
		assert.Equal(t, "trace-456", result.TraceID)
		assert.Equal(t, []byte(`{"key": "value"}`), result.Body)
		assert.Len(t, result.Headers, 2)
	})

	t.Run("empty timeout", func(t *testing.T) {
		dto := &RelayRequest{
			URL: "https://example.com",
		}

		result, err := dto.ToProtocolRequest()
		require.NoError(t, err)
		assert.Equal(t, time.Duration(0), result.Timeout)
	})

	t.Run("invalid timeout", func(t *testing.T) {
		dto := &RelayRequest{
			URL:     "https://example.com",
			Timeout: "invalid",
		}

		_, err := dto.ToProtocolRequest()
		assert.Error(t, err)
	})
}

func TestFromProtocolResponse(t *testing.T) {
	t.Run("full response", func(t *testing.T) {
		resp := &protocol.Response{
			RequestID:  "req-123",
			StatusCode: 200,
			Headers: protocol.HeaderMap{
				{Key: "Content-Type", Value: "application/json"},
				{Key: "X-Custom", Value: "value"},
			},
			Body:      []byte(`{"success": true}`),
			SessionID: "sess-456",
			Timing: &protocol.TimingInfo{
				DNSLookup:    10 * time.Millisecond,
				TCPConnect:   20 * time.Millisecond,
				TLSHandshake: 30 * time.Millisecond,
				FirstByte:    50 * time.Millisecond,
				Total:        100 * time.Millisecond,
			},
		}

		meta := &RelayMetaDTO{
			Retries:    2,
			Pool:       "1",
			EndpointID: "ep-789",
		}

		result := FromProtocolResponse(resp, meta)

		assert.Equal(t, "req-123", result.RequestID)
		assert.Equal(t, 200, result.StatusCode)
		assert.Equal(t, "application/json", result.Headers["Content-Type"])
		assert.Equal(t, "value", result.Headers["X-Custom"])
		assert.Equal(t, []byte(`{"success": true}`), result.Body)
		assert.Equal(t, "sess-456", result.SessionID)
		assert.NotNil(t, result.Timing)
		assert.Equal(t, "10ms", result.Timing.DNSLookup)
		assert.Equal(t, "100ms", result.Timing.Total)
		assert.Equal(t, 2, result.Meta.Retries)
	})

	t.Run("nil timing", func(t *testing.T) {
		resp := &protocol.Response{
			RequestID:  "req-123",
			StatusCode: 200,
		}

		result := FromProtocolResponse(resp, nil)

		assert.Nil(t, result.Timing)
		assert.Nil(t, result.Meta)
	})
}

func TestCreateRoutingRuleRequest_ToDomain(t *testing.T) {
	t.Run("full request", func(t *testing.T) {
		dto := &CreateRoutingRuleRequest{
			Name:                 "Test Rule",
			RequiredTags:         []string{"target:amazon"},
			ExcludedTags:         []string{"type:blocked"},
			Priority:             100,
			HardTimeout:          "1m30s",
			RateLimitPerMinute:   60,
			RateLimitPerSecond:   5,
			AllowedEndpointTypes: []string{"residential"},
			FingerprintPreset:    "chrome-130",
			FingerprintABTest: &ABConfigDTO{
				Strategy: "weighted",
				Variants: []ABVariantDTO{
					{Fingerprint: "chrome-130", Weight: 80},
					{Fingerprint: "firefox-120", Weight: 20},
				},
			},
			RequestFilters: &RequestFilterDTO{
				BlockDomains:  []string{"ads.example.com"},
				EnableAdblock: true,
			},
			EndpointPools: []EndpointPoolDTO{
				{Tier: 1, Endpoints: []string{"ep-1", "ep-2"}, MaxRetries: 3},
			},
			IsActive: true,
		}

		result, err := dto.ToDomain()
		require.NoError(t, err)

		assert.Equal(t, "Test Rule", result.Name)
		assert.Equal(t, []string{"target:amazon"}, result.RequiredTags)
		assert.Equal(t, 100, result.Priority)
		assert.Equal(t, 90*time.Second, result.HardTimeout)
		assert.Equal(t, 60, result.RateLimitPerMinute)
		assert.Equal(t, "chrome-130", result.FingerprintPreset)
		assert.NotNil(t, result.FingerprintABTest)
		assert.Equal(t, 2, len(result.FingerprintABTest.Variants))
		assert.NotNil(t, result.RequestFilters)
		assert.True(t, result.RequestFilters.EnableAdblock)
		assert.Len(t, result.EndpointPools, 1)
		assert.True(t, result.IsActive)
	})

	t.Run("invalid timeout", func(t *testing.T) {
		dto := &CreateRoutingRuleRequest{
			Name:        "Test",
			HardTimeout: "invalid",
		}

		_, err := dto.ToDomain()
		assert.Error(t, err)
	})
}

func TestFromRoutingRule(t *testing.T) {
	t.Run("full rule", func(t *testing.T) {
		now := time.Now()
		rule := &domain.RoutingRule{
			ID:                 "rule-123",
			Name:               "Test Rule",
			RequiredTags:       []string{"target:amazon"},
			Priority:           100,
			HardTimeout:        90 * time.Second,
			RateLimitPerMinute: 60,
			FingerprintPreset:  "chrome-130",
			FingerprintABTest: &domain.ABConfig{
				Strategy: "random",
				Variants: []domain.ABVariant{
					{Fingerprint: "chrome-130", Weight: 50},
				},
			},
			RequestFilters: &domain.RequestFilter{
				EnableAdblock: true,
			},
			EndpointPools: []domain.EndpointPool{
				{Tier: 1, Endpoints: []string{"ep-1"}, MaxRetries: 2},
			},
			IsActive:  true,
			Version:   5,
			CreatedAt: now,
			UpdatedAt: now,
		}

		result := FromRoutingRule(rule)

		assert.Equal(t, "rule-123", result.ID)
		assert.Equal(t, "Test Rule", result.Name)
		assert.Equal(t, "1m30s", result.HardTimeout)
		assert.Equal(t, 100, result.Priority)
		assert.NotNil(t, result.FingerprintABTest)
		assert.NotNil(t, result.RequestFilters)
		assert.Len(t, result.EndpointPools, 1)
		assert.Equal(t, 5, result.Version)
	})

	t.Run("nil rule", func(t *testing.T) {
		result := FromRoutingRule(nil)
		assert.Nil(t, result)
	})
}

func TestFromApiKey(t *testing.T) {
	t.Run("full key", func(t *testing.T) {
		now := time.Now()
		expires := now.Add(24 * time.Hour)
		rateLimit := 100

		key := &domain.ApiKey{
			ID:                "key-123",
			TokenHash:         "secret-hash-should-not-leak", // This should NOT appear in response
			Name:              "Test Key",
			Scopes:            []string{"target:*"},
			RateLimitOverride: &rateLimit,
			IsActive:          true,
			CreatedAt:         now,
			ExpiresAt:         &expires,
		}

		result := FromApiKey(key)

		assert.Equal(t, "key-123", result.ID)
		assert.Equal(t, "Test Key", result.Name)
		assert.Equal(t, []string{"target:*"}, result.Scopes)
		assert.Equal(t, &rateLimit, result.RateLimitOverride)
		assert.True(t, result.IsActive)
		assert.Equal(t, now, result.CreatedAt)
		assert.Equal(t, &expires, result.ExpiresAt)
	})

	t.Run("nil key", func(t *testing.T) {
		result := FromApiKey(nil)
		assert.Nil(t, result)
	})
}

func TestFromApiKeys(t *testing.T) {
	keys := []domain.ApiKey{
		{ID: "key-1", Name: "Key 1", IsActive: true},
		{ID: "key-2", Name: "Key 2", IsActive: false},
	}

	result := FromApiKeys(keys)

	assert.Len(t, result, 2)
	assert.Equal(t, "key-1", result[0].ID)
	assert.Equal(t, "key-2", result[1].ID)
}

func TestCreateFingerprintRequest_ToDomain(t *testing.T) {
	dto := &CreateFingerprintRequest{
		ID:   "chrome-130",
		Name: "Chrome 130",
		Config: map[string]interface{}{
			"user_agent": "Mozilla/5.0...",
			"tls":        map[string]interface{}{"version": "1.3"},
		},
	}

	result := dto.ToDomain()

	assert.Equal(t, "chrome-130", result.ID)
	assert.Equal(t, "Chrome 130", result.Name)
	assert.Equal(t, "Mozilla/5.0...", result.Config["user_agent"])
}

func TestFromFingerprintPreset(t *testing.T) {
	now := time.Now()
	preset := &domain.FingerprintPreset{
		ID:        "chrome-130",
		Name:      "Chrome 130",
		Config:    domain.ConfigMap{"tls_version": "1.3"},
		CreatedAt: now,
		UpdatedAt: now,
	}

	result := FromFingerprintPreset(preset)

	assert.Equal(t, "chrome-130", result.ID)
	assert.Equal(t, "Chrome 130", result.Name)
	assert.Equal(t, "1.3", result.Config["tls_version"])
	assert.Equal(t, now, result.CreatedAt)

	// Test nil
	assert.Nil(t, FromFingerprintPreset(nil))
}

func TestFromUsageSummary(t *testing.T) {
	summary := &domain.UsageSummary{
		Date:          "2026-01-05",
		TotalRequests: 1000,
		TotalBytes:    1024000,
		CostUnits:     10.5,
		Breakdown:     map[string]int64{"rule-1": 500, "rule-2": 500},
	}

	result := FromUsageSummary(summary)

	assert.Equal(t, "2026-01-05", result.Date)
	assert.Equal(t, int64(1000), result.TotalRequests)
	assert.Equal(t, int64(1024000), result.TotalBytes)
	assert.Equal(t, 10.5, result.CostUnits)
	assert.Len(t, result.Breakdown, 2)

	// Test nil
	assert.Nil(t, FromUsageSummary(nil))
}

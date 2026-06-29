package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testTagsTarget = "target"
	testTagsType   = "type"
	testTagsAmazon = "amazon"
	testTagsSearch = "search"
	testTagsRegion = "region"
)

func TestParseTags(t *testing.T) {
	parser := NewTagParser()

	tests := []struct {
		name          string
		headers       map[string]string
		apiKey        *domain.APIKey
		expectedTags  []domain.Tag
		expectedWarns int
		expectError   bool
	}{
		{
			name: "Basic X-Relay-Tags",
			headers: map[string]string{
				HeaderRelayTags: "target:amazon, type:search",
			},
			expectedTags: []domain.Tag{
				{Key: "target", Value: testTagsAmazon},
				{Key: "type", Value: testTagsSearch},
			},
		},
		{
			name: "X-Relay-Tags with spaces and equals",
			headers: map[string]string{
				HeaderRelayTags: "target=google,  capability: stealth ",
			},
			expectedTags: []domain.Tag{
				{Key: testTagsTarget, Value: "google"},
				{Key: "capability", Value: "stealth"},
			},
		},
		{
			name: "Legacy Headers",
			headers: map[string]string{
				HeaderLegacyRetailer: "amazon",
				HeaderLegacyMode:     "search",
				HeaderLegacyCountry:  "us",
			},
			expectedTags: []domain.Tag{
				{Key: "target", Value: testTagsAmazon},
				{Key: "type", Value: testTagsSearch},
				{Key: testTagsRegion, Value: "us"},
			},
			expectedWarns: 3,
		},
		{
			name: "Mixed Sources (Legacy + Modern)",
			headers: map[string]string{
				HeaderRelayTags:      "capability:fast",
				HeaderLegacyRetailer: testTagsAmazon,
			},
			expectedTags: []domain.Tag{
				{Key: "capability", Value: "fast"},
				{Key: testTagsTarget, Value: testTagsAmazon},
			},
			expectedWarns: 1,
		},
		{
			name: "API Key Scopes",
			apiKey: &domain.APIKey{
				Scopes: []string{"customer:vip", "tier:gold"},
			},
			expectedTags: []domain.Tag{
				{Key: "customer", Value: "vip"},
				{Key: "tier", Value: "gold"},
			},
		},
		{
			name: "All Sources Merge & Deduplicate",
			headers: map[string]string{
				HeaderRelayTags:     "target:amazon, type:search",
				HeaderLegacyCountry: "us",
			},
			apiKey: &domain.APIKey{
				Scopes: []string{"customer:vip", "target:" + testTagsAmazon},
			},
			expectedTags: []domain.Tag{
				{Key: testTagsTarget, Value: testTagsAmazon},
				{Key: testTagsType, Value: testTagsSearch},
				{Key: testTagsRegion, Value: "us"},
				{Key: "customer", Value: "vip"},
			},
			expectedWarns: 1,
		},
		{
			name: "Invalid Format",
			headers: map[string]string{
				HeaderRelayTags: "invalid-tag-format",
			},
			expectError: true,
		},
		{
			name: "Invalid API Key Scopes",
			apiKey: &domain.APIKey{
				Scopes: []string{"invalid-scope-format"},
			},
			expectError: true,
		},
		{
			name:         "Empty Request - No Tags",
			headers:      map[string]string{},
			apiKey:       nil,
			expectedTags: []domain.Tag{},
		},
		{
			name: "X-Relay-Tags with Empty Value",
			headers: map[string]string{
				HeaderRelayTags: "",
			},
			apiKey:       nil,
			expectedTags: []domain.Tag{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result, err := parser.ParseTags(req, tt.apiKey)
			if tt.expectError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Len(t, result.Warnings, tt.expectedWarns)

			assert.ElementsMatch(t, tt.expectedTags, result.Tags)
		})
	}
}

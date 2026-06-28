package ratelimit_test

import (
	"testing"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/service/ratelimit"
	"github.com/stretchr/testify/assert"
)

func TestGenerateQuotaKey(t *testing.T) {
	tests := []struct {
		name     string
		rule     *domain.RoutingRule
		apiKeyID string
		want     string
	}{
		{
			name: "Uses QuotaKey from rule",
			rule: &domain.RoutingRule{
				ID:       "rule-123",
				QuotaKey: "target:amazon",
			},
			apiKeyID: "key-abc",
			want:     "target:amazon:key-abc",
		},
		{
			name: "Falls back to Rule ID",
			rule: &domain.RoutingRule{
				ID: "rule-123",
			},
			apiKeyID: "key-abc",
			want:     "rule-123:key-abc",
		},
		{
			name: "Handles empty QuotaKey with Rule ID fallback",
			rule: &domain.RoutingRule{
				ID:       "rule-456",
				QuotaKey: "",
			},
			apiKeyID: "key-xyz",
			want:     "rule-456:key-xyz",
		},
		{
			name: "Handles empty apiKeyID",
			rule: &domain.RoutingRule{
				ID:       "rule-789",
				QuotaKey: "target:ebay",
			},
			apiKeyID: "",
			want:     "target:ebay:",
		},
		{
			name: "Handles empty QuotaKey and empty apiKeyID",
			rule: &domain.RoutingRule{
				ID:       "rule-empty",
				QuotaKey: "",
			},
			apiKeyID: "",
			want:     "rule-empty:",
		},
		{
			name: "Handles special characters in QuotaKey",
			rule: &domain.RoutingRule{
				ID:       "rule-special",
				QuotaKey: "target:amazon.com:us-west",
			},
			apiKeyID: "key-with-dashes",
			want:     "target:amazon.com:us-west:key-with-dashes",
		},
		{
			name: "Handles special characters in Rule ID",
			rule: &domain.RoutingRule{
				ID: "rule-with_underscores-and.dots",
			},
			apiKeyID: "key-with/slashes",
			want:     "rule-with_underscores-and.dots:key-with/slashes",
		},
		{
			name: "Handles UUID-style IDs",
			rule: &domain.RoutingRule{
				ID:       "550e8400-e29b-41d4-a716-446655440000",
				QuotaKey: "quota:custom",
			},
			apiKeyID: "660e8400-e29b-41d4-a716-446655440001",
			want:     "quota:custom:660e8400-e29b-41d4-a716-446655440001",
		},
		{
			name: "Handles numeric IDs",
			rule: &domain.RoutingRule{
				ID:       "12345",
				QuotaKey: "45678",
			},
			apiKeyID: "99999",
			want:     "45678:99999",
		},
		{
			name: "Handles long QuotaKey",
			rule: &domain.RoutingRule{
				ID:       "rule-long",
				QuotaKey: "very:long:quota:key:with:many:colons:and:segments",
			},
			apiKeyID: "api-key-id",
			want:     "very:long:quota:key:with:many:colons:and:segments:api-key-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ratelimit.GenerateQuotaKey(tt.rule, tt.apiKeyID)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateQuotaKey_NilRule(t *testing.T) {

	t.Run("Panics with nil rule", func(t *testing.T) {
		assert.Panics(t, func() {
			ratelimit.GenerateQuotaKey(nil, "key-abc")
		})
	})
}

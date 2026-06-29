package ratelimit

import (
	"fmt"

	"github.com/beremaran/straw/internal/domain"
)

// GenerateQuotaKey builds a rate limit key from the rule's QuotaKey (or Rule ID) and the API key ID.
func GenerateQuotaKey(rule *domain.RoutingRule, apiKeyID string) string {
	prefix := rule.QuotaKey
	if prefix == "" {
		prefix = rule.ID
	}

	return fmt.Sprintf("%s:%s", prefix, apiKeyID)
}

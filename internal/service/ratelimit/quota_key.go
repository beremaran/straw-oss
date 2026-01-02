package ratelimit

import (
	"fmt"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// GenerateQuotaKey derives the rate limit bucket key from the routing rule and API key.
// Format: quota:{rule_quota_key_or_id}:{api_key_id}
func GenerateQuotaKey(rule *domain.RoutingRule, apiKeyID string) string {
	prefix := rule.QuotaKey
	if prefix == "" {
		prefix = rule.ID
	}
	return fmt.Sprintf("%s:%s", prefix, apiKeyID)
}

package ratelimit

import (
	"fmt"

	"github.com/beremaran/straw/internal/domain"
)

func GenerateQuotaKey(rule *domain.RoutingRule, apiKeyID string) string {
	prefix := rule.QuotaKey
	if prefix == "" {
		prefix = rule.ID
	}
	return fmt.Sprintf("%s:%s", prefix, apiKeyID)
}

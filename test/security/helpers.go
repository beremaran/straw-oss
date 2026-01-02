package security

import (
	"context"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// Dummy Implementations to satisfy interfaces

type dummySelector struct{}

func (d *dummySelector) Select(ctx context.Context, rule *domain.RoutingRule) (string, error) {
	return "dummy-endpoint", nil
}
func (d *dummySelector) SelectWithSession(ctx context.Context, sessionID string) (string, error) {
	return "dummy-endpoint", nil
}

type dummyPoolManager struct{}

func (d *dummyPoolManager) GetEndpointFromPool(ctx context.Context, rule *domain.RoutingRule, poolTier int, exclude []string) (string, error) {
	return "dummy-endpoint", nil
}
func (d *dummyPoolManager) GetPoolConfig(rule *domain.RoutingRule, poolTier int) *domain.EndpointPool {
	if len(rule.EndpointPools) > 0 {
		return &rule.EndpointPools[0]
	}
	return nil
}

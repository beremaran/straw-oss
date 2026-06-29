// Package security provides test helpers for security tests.
package security

import (
	"context"

	"github.com/beremaran/straw/internal/domain"
)

const dummyEndpoint = "dummy-endpoint"

type dummySelector struct{}

func (d *dummySelector) Select(_ context.Context, _ *domain.RoutingRule) (string, error) {
	return dummyEndpoint, nil
}

func (d *dummySelector) SelectWithSession(_ context.Context, _ string) (string, error) {
	return dummyEndpoint, nil
}

type dummyPoolManager struct{}

func (d *dummyPoolManager) GetEndpointFromPool(_ context.Context, _ *domain.RoutingRule, _ int, _ []string) (string, error) {
	return dummyEndpoint, nil
}

func (d *dummyPoolManager) GetPoolConfig(rule *domain.RoutingRule, _ int) *domain.EndpointPool {
	if len(rule.EndpointPools) > 0 {
		return &rule.EndpointPools[0]
	}

	return nil
}

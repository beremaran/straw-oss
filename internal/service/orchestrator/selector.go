package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

// SimpleEndpointSelector implements EndpointSelector using simple strategies.
type SimpleEndpointSelector struct {
	healthStore *redis.EndpointHealthStore
}

// NewSimpleEndpointSelector creates a new SimpleEndpointSelector.
func NewSimpleEndpointSelector(healthStore *redis.EndpointHealthStore) *SimpleEndpointSelector {
	return &SimpleEndpointSelector{
		healthStore: healthStore,
	}
}

// Select chooses an endpoint based on the routing rule.
func (s *SimpleEndpointSelector) Select(ctx context.Context, rule *domain.RoutingRule) (string, error) {
	// Use pool tier 1 (primary) by default for initial selection
	return s.GetEndpointFromPool(ctx, rule, 1, nil)
}

// SelectWithSession chooses an endpoint based on an existing session.
func (s *SimpleEndpointSelector) SelectWithSession(ctx context.Context, sessionID string) (string, error) {
	return "", errors.New("method SelectWithSession not implemented: requires session store access")
}

// GetEndpointFromPool selects an endpoint from the specified pool tier.
func (s *SimpleEndpointSelector) GetEndpointFromPool(ctx context.Context, rule *domain.RoutingRule, poolTier int, exclude []string) (string, error) {
	poolConfig := s.GetPoolConfig(rule, poolTier)
	var requiredTags []string
	if poolConfig != nil {
		requiredTags = rule.RequiredTags
	} else {
		if poolTier > 1 {
			// If we are looking for a non-primary pool, and it's not configured, return error
			// unless we decide implicit pools exist (e.g. same tags, different subset).
			// For now, strict: must be configured.
			// However, the rule might not have *any* pools defined, implying just one default pool.
			if len(rule.EndpointPools) == 0 {
				return "", fmt.Errorf("pool tier %d not configured", poolTier)
			}
		}
		requiredTags = rule.RequiredTags
	}

	endpoints, err := s.healthStore.ListHealthyByTags(ctx, requiredTags)
	if err != nil {
		return "", fmt.Errorf("failed to list healthy endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		return "", fmt.Errorf("no healthy endpoints found for tags: %v", requiredTags)
	}

	// Filter by explicit endpoints if configured in the pool
	if poolConfig != nil && len(poolConfig.Endpoints) > 0 {
		allowedEndpoints := make(map[string]bool)
		for _, id := range poolConfig.Endpoints {
			allowedEndpoints[id] = true
		}

		filteredEndpoints := make([]*redis.EndpointHealth, 0, len(endpoints))
		for _, ep := range endpoints {
			if allowedEndpoints[ep.EndpointID] {
				filteredEndpoints = append(filteredEndpoints, ep)
			}
		}
		endpoints = filteredEndpoints

		if len(endpoints) == 0 {
			return "", fmt.Errorf("no healthy endpoints found in configured pool (tier %d)", poolTier)
		}
	}

	// Filter out excluded endpoints
	candidates := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		excluded := false
		for _, ex := range exclude {
			if ep.EndpointID == ex {
				excluded = true
				break
			}
		}
		if !excluded {
			candidates = append(candidates, ep.EndpointID)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no available endpoints after exclusion")
	}

	idx := rand.Intn(len(candidates))
	return candidates[idx], nil
}

// GetPoolConfig returns the configuration for the specified pool tier.
func (s *SimpleEndpointSelector) GetPoolConfig(rule *domain.RoutingRule, poolTier int) *domain.EndpointPool {
	if rule == nil {
		return nil
	}
	for i := range rule.EndpointPools {
		if rule.EndpointPools[i].Tier == poolTier {
			return &rule.EndpointPools[i]
		}
	}
	return nil
}

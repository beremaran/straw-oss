package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/internal/infra/redis"
)

var (
	ErrSelectWithSessionNotImplemented    = errors.New("method SelectWithSession not implemented: requires session store access")
	ErrPoolTierNotConfigured              = errors.New("pool tier not configured")
	ErrNoHealthyEndpoints                 = errors.New("no healthy endpoints found")
	ErrNoHealthyEndpointsInPool           = errors.New("no healthy endpoints found in configured pool")
	ErrNoAvailableEndpointsAfterExclusion = errors.New("no available endpoints after exclusion")
)

type SimpleEndpointSelector struct {
	healthStore *redis.EndpointHealthStore
}

func NewSimpleEndpointSelector(healthStore *redis.EndpointHealthStore) *SimpleEndpointSelector {
	return &SimpleEndpointSelector{
		healthStore: healthStore,
	}
}

func (s *SimpleEndpointSelector) Select(ctx context.Context, rule *domain.RoutingRule) (string, error) {
	return s.GetEndpointFromPool(ctx, rule, 1, nil)
}

func (s *SimpleEndpointSelector) SelectWithSession(ctx context.Context, sessionID string) (string, error) {
	return "", ErrSelectWithSessionNotImplemented
}

func (s *SimpleEndpointSelector) GetEndpointFromPool(ctx context.Context, rule *domain.RoutingRule, poolTier int, exclude []string) (string, error) {
	poolConfig := s.GetPoolConfig(rule, poolTier)
	requiredTags, err := requiredTagsForPool(rule, poolConfig, poolTier)
	if err != nil {
		return "", err
	}

	endpoints, err := s.healthStore.ListHealthyByTags(ctx, requiredTags)
	if err != nil {
		return "", fmt.Errorf("failed to list healthy endpoints: %w", err)
	}

	if len(endpoints) == 0 {
		return "", fmt.Errorf("%w for tags: %v", ErrNoHealthyEndpoints, requiredTags)
	}

	endpoints, err = filterPoolEndpoints(endpoints, poolConfig, poolTier)
	if err != nil {
		return "", err
	}

	candidates := endpointCandidates(endpoints, exclude)
	if len(candidates) == 0 {
		return "", ErrNoAvailableEndpointsAfterExclusion
	}

	idx := rand.Intn(len(candidates))

	return candidates[idx], nil
}

func requiredTagsForPool(
	rule *domain.RoutingRule,
	poolConfig *domain.EndpointPool,
	poolTier int,
) ([]string, error) {
	if poolConfig == nil && poolTier > 1 && len(rule.EndpointPools) == 0 {
		return nil, fmt.Errorf("%w: tier %d", ErrPoolTierNotConfigured, poolTier)
	}

	return rule.RequiredTags, nil
}

func filterPoolEndpoints(
	endpoints []*redis.EndpointHealth,
	poolConfig *domain.EndpointPool,
	poolTier int,
) ([]*redis.EndpointHealth, error) {
	if poolConfig == nil || len(poolConfig.Endpoints) == 0 {
		return endpoints, nil
	}

	allowedEndpoints := make(map[string]bool, len(poolConfig.Endpoints))
	for _, id := range poolConfig.Endpoints {
		allowedEndpoints[id] = true
	}

	filteredEndpoints := make([]*redis.EndpointHealth, 0, len(endpoints))
	for _, ep := range endpoints {
		if allowedEndpoints[ep.EndpointID] {
			filteredEndpoints = append(filteredEndpoints, ep)
		}
	}

	if len(filteredEndpoints) == 0 {
		return nil, fmt.Errorf("%w (tier %d)", ErrNoHealthyEndpointsInPool, poolTier)
	}

	return filteredEndpoints, nil
}

func endpointCandidates(endpoints []*redis.EndpointHealth, exclude []string) []string {
	excluded := make(map[string]bool, len(exclude))
	for _, endpointID := range exclude {
		excluded[endpointID] = true
	}

	candidates := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		if !excluded[ep.EndpointID] {
			candidates = append(candidates, ep.EndpointID)
		}
	}

	return candidates
}

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

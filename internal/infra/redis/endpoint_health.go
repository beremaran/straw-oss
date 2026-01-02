// Package redis provides Redis-backed implementations for various stores.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Health state constants
const (
	HealthStateHealthy   = "healthy"
	HealthStateSuspect   = "suspect"
	HealthStateUnhealthy = "unhealthy"
	HealthStateDraining  = "draining"
)

// Redis key prefixes for endpoint health
const (
	endpointHealthKeyPrefix = "endpoint:health:"
	endpointHealthIndexKey  = "endpoint:health:index"
	endpointDrainingSetKey  = "endpoint:draining"
)

// Default health TTL for Redis keys (60s, double the unhealthy threshold)
const defaultHealthTTL = 60 * time.Second

// EndpointHealth represents the health state of an endpoint in Redis.
type EndpointHealth struct {
	// EndpointID is the unique identifier for this endpoint.
	EndpointID string `json:"endpoint_id"`

	// State is the health state: "healthy", "suspect", "unhealthy", or "draining".
	State string `json:"state"`

	// Tags describe the endpoint's capabilities.
	Tags []string `json:"tags,omitempty"`

	// Version is the endpoint software version.
	Version string `json:"version,omitempty"`

	// ActiveTasks is the number of currently processing tasks.
	ActiveTasks int `json:"active_tasks"`

	// LastSeen is when the last heartbeat was received.
	LastSeen time.Time `json:"last_seen"`
}

// HealthStore defines the interface for endpoint health persistence.
type HealthStore interface {
	// UpdateHealth updates the health state for an endpoint.
	UpdateHealth(ctx context.Context, health *EndpointHealth) error

	// GetHealth retrieves the health state for an endpoint.
	GetHealth(ctx context.Context, endpointID string) (*EndpointHealth, error)

	// ListHealthyByTags returns healthy endpoints matching all given tags.
	ListHealthyByTags(ctx context.Context, tags []string) ([]*EndpointHealth, error)

	// ListAllEndpoints returns all known endpoints (for staleness checking).
	ListAllEndpoints(ctx context.Context) ([]*EndpointHealth, error)

	// DeleteHealth removes an endpoint's health record.
	DeleteHealth(ctx context.Context, endpointID string) error

	// SetDraining sets the draining state for an endpoint.
	SetDraining(ctx context.Context, endpointID string, draining bool) error

	// IsDraining checks if an endpoint is in draining state.
	IsDraining(ctx context.Context, endpointID string) (bool, error)
}

// EndpointHealthStore implements HealthStore using Redis.
type EndpointHealthStore struct {
	client *Client
}

// NewEndpointHealthStore creates a new EndpointHealthStore.
func NewEndpointHealthStore(client *Client) *EndpointHealthStore {
	return &EndpointHealthStore{client: client}
}

// healthKey returns the Redis key for an endpoint's health record.
func healthKey(endpointID string) string {
	return endpointHealthKeyPrefix + endpointID
}

// UpdateHealth updates the health state for an endpoint.
func (s *EndpointHealthStore) UpdateHealth(ctx context.Context, health *EndpointHealth) error {
	if health == nil {
		return fmt.Errorf("health cannot be nil")
	}
	if health.EndpointID == "" {
		return fmt.Errorf("endpoint_id cannot be empty")
	}

	data, err := json.Marshal(health)
	if err != nil {
		return fmt.Errorf("failed to marshal health: %w", err)
	}

	key := healthKey(health.EndpointID)

	// Use pipeline to set health data and update index atomically
	pipe := s.client.Client.Pipeline()
	pipe.Set(ctx, key, data, defaultHealthTTL)
	pipe.ZAdd(ctx, endpointHealthIndexKey, redis.Z{
		Score:  float64(health.LastSeen.Unix()),
		Member: health.EndpointID,
	})

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update health: %w", err)
	}

	return nil
}

// GetHealth retrieves the health state for an endpoint.
func (s *EndpointHealthStore) GetHealth(ctx context.Context, endpointID string) (*EndpointHealth, error) {
	if endpointID == "" {
		return nil, fmt.Errorf("endpoint_id cannot be empty")
	}

	key := healthKey(endpointID)
	data, err := s.client.Client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("failed to get health: %w", err)
	}

	var health EndpointHealth
	if err := json.Unmarshal(data, &health); err != nil {
		return nil, fmt.Errorf("failed to unmarshal health: %w", err)
	}

	return &health, nil
}

// ListHealthyByTags returns healthy endpoints matching all given tags.
func (s *EndpointHealthStore) ListHealthyByTags(ctx context.Context, tags []string) ([]*EndpointHealth, error) {
	// Get all endpoint IDs from the index
	endpointIDs, err := s.client.Client.ZRange(ctx, endpointHealthIndexKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoint IDs: %w", err)
	}

	if len(endpointIDs) == 0 {
		return []*EndpointHealth{}, nil
	}

	// Build keys for all endpoints
	keys := make([]string, len(endpointIDs))
	for i, id := range endpointIDs {
		keys[i] = healthKey(id)
	}

	// Fetch all health records
	results, err := s.client.Client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get health records: %w", err)
	}

	healthy := make([]*EndpointHealth, 0)
	for _, result := range results {
		if result == nil {
			continue
		}

		str, ok := result.(string)
		if !ok {
			continue
		}

		var health EndpointHealth
		if err := json.Unmarshal([]byte(str), &health); err != nil {
			continue
		}

		// Only include healthy or suspect endpoints
		if health.State != HealthStateHealthy && health.State != HealthStateSuspect {
			continue
		}

		// Check if endpoint has all required tags
		if matchesTags(&health, tags) {
			healthy = append(healthy, &health)
		}
	}

	return healthy, nil
}

// ListAllEndpoints returns all known endpoints.
func (s *EndpointHealthStore) ListAllEndpoints(ctx context.Context) ([]*EndpointHealth, error) {
	// Get all endpoint IDs from the index
	endpointIDs, err := s.client.Client.ZRange(ctx, endpointHealthIndexKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoint IDs: %w", err)
	}

	if len(endpointIDs) == 0 {
		return []*EndpointHealth{}, nil
	}

	// Build keys for all endpoints
	keys := make([]string, len(endpointIDs))
	for i, id := range endpointIDs {
		keys[i] = healthKey(id)
	}

	// Fetch all health records
	results, err := s.client.Client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get health records: %w", err)
	}

	endpoints := make([]*EndpointHealth, 0, len(results))
	for _, result := range results {
		if result == nil {
			continue
		}

		str, ok := result.(string)
		if !ok {
			continue
		}

		var health EndpointHealth
		if err := json.Unmarshal([]byte(str), &health); err != nil {
			continue
		}

		endpoints = append(endpoints, &health)
	}

	return endpoints, nil
}

// DeleteHealth removes an endpoint's health record.
func (s *EndpointHealthStore) DeleteHealth(ctx context.Context, endpointID string) error {
	if endpointID == "" {
		return fmt.Errorf("endpoint_id cannot be empty")
	}

	key := healthKey(endpointID)

	// Use pipeline to delete health data and remove from index
	pipe := s.client.Client.Pipeline()
	pipe.Del(ctx, key)
	pipe.ZRem(ctx, endpointHealthIndexKey, endpointID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete health: %w", err)
	}

	return nil
}

// matchesTags checks if the endpoint has all required tags.
func matchesTags(health *EndpointHealth, requiredTags []string) bool {
	if len(requiredTags) == 0 {
		return true
	}

	tagSet := make(map[string]struct{}, len(health.Tags))
	for _, tag := range health.Tags {
		tagSet[tag] = struct{}{}
	}

	for _, required := range requiredTags {
		if _, ok := tagSet[required]; !ok {
			return false
		}
	}

	return true
}

// SetDraining sets the draining state for an endpoint.
func (s *EndpointHealthStore) SetDraining(ctx context.Context, endpointID string, draining bool) error {
	if endpointID == "" {
		return fmt.Errorf("endpoint_id cannot be empty")
	}

	if draining {
		return s.client.Client.SAdd(ctx, endpointDrainingSetKey, endpointID).Err()
	}
	return s.client.Client.SRem(ctx, endpointDrainingSetKey, endpointID).Err()
}

// IsDraining checks if an endpoint is in draining state.
func (s *EndpointHealthStore) IsDraining(ctx context.Context, endpointID string) (bool, error) {
	if endpointID == "" {
		return false, fmt.Errorf("endpoint_id cannot be empty")
	}

	return s.client.Client.SIsMember(ctx, endpointDrainingSetKey, endpointID).Result()
}

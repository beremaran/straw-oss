package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	HealthStateHealthy   = "healthy"
	HealthStateSuspect   = "suspect"
	HealthStateUnhealthy = "unhealthy"
	HealthStateDraining  = "draining"
)

const (
	endpointHealthKeyPrefix = "endpoint:health:"
	endpointHealthIndexKey  = "endpoint:health:index"
	endpointDrainingSetKey  = "endpoint:draining"
)

const defaultHealthTTL = 60 * time.Second

type EndpointHealth struct {
	EndpointID string `json:"endpoint_id"`

	State string `json:"state"`

	Tags []string `json:"tags,omitempty"`

	Version string `json:"version,omitempty"`

	ActiveTasks int `json:"active_tasks"`

	LastSeen time.Time `json:"last_seen"`
}

type HealthStore interface {
	UpdateHealth(ctx context.Context, health *EndpointHealth) error

	GetHealth(ctx context.Context, endpointID string) (*EndpointHealth, error)

	ListHealthyByTags(ctx context.Context, tags []string) ([]*EndpointHealth, error)

	ListAllEndpoints(ctx context.Context) ([]*EndpointHealth, error)

	DeleteHealth(ctx context.Context, endpointID string) error

	SetDraining(ctx context.Context, endpointID string, draining bool) error

	IsDraining(ctx context.Context, endpointID string) (bool, error)
}

type EndpointHealthStore struct {
	client *Client
}

func NewEndpointHealthStore(client *Client) *EndpointHealthStore {
	return &EndpointHealthStore{client: client}
}

func healthKey(endpointID string) string {
	return endpointHealthKeyPrefix + endpointID
}

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

func (s *EndpointHealthStore) GetHealth(ctx context.Context, endpointID string) (*EndpointHealth, error) {
	if endpointID == "" {
		return nil, fmt.Errorf("endpoint_id cannot be empty")
	}

	key := healthKey(endpointID)
	data, err := s.client.Client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
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

func (s *EndpointHealthStore) ListHealthyByTags(ctx context.Context, tags []string) ([]*EndpointHealth, error) {
	endpointIDs, err := s.client.Client.ZRange(ctx, endpointHealthIndexKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoint IDs: %w", err)
	}

	if len(endpointIDs) == 0 {
		return []*EndpointHealth{}, nil
	}

	keys := make([]string, len(endpointIDs))
	for i, id := range endpointIDs {
		keys[i] = healthKey(id)
	}

	results, err := s.client.Client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get health records: %w", err)
	}

	healthy := make([]*EndpointHealth, 0)
	for _, result := range results {
		h, ok := parseHealthRecord(result)
		if !ok {
			continue
		}

		if !isEligible(h, tags) {
			continue
		}

		healthy = append(healthy, h)
	}

	return healthy, nil
}

func (s *EndpointHealthStore) ListAllEndpoints(ctx context.Context) ([]*EndpointHealth, error) {
	endpointIDs, err := s.client.Client.ZRange(ctx, endpointHealthIndexKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list endpoint IDs: %w", err)
	}

	if len(endpointIDs) == 0 {
		return []*EndpointHealth{}, nil
	}

	keys := make([]string, len(endpointIDs))
	for i, id := range endpointIDs {
		keys[i] = healthKey(id)
	}

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
		err := json.Unmarshal([]byte(str), &health)
		if err != nil {
			continue
		}

		endpoints = append(endpoints, &health)
	}

	return endpoints, nil
}

func (s *EndpointHealthStore) DeleteHealth(ctx context.Context, endpointID string) error {
	if endpointID == "" {
		return fmt.Errorf("endpoint_id cannot be empty")
	}

	key := healthKey(endpointID)

	pipe := s.client.Client.Pipeline()
	pipe.Del(ctx, key)
	pipe.ZRem(ctx, endpointHealthIndexKey, endpointID)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete health: %w", err)
	}

	return nil
}

func parseHealthRecord(result any) (*EndpointHealth, bool) {
	if result == nil {
		return nil, false
	}
	str, ok := result.(string)
	if !ok {
		return nil, false
	}
	var health EndpointHealth
	if err := json.Unmarshal([]byte(str), &health); err != nil {
		return nil, false
	}
	return &health, true
}

func isEligible(health *EndpointHealth, tags []string) bool {
	if health.State != HealthStateHealthy && health.State != HealthStateSuspect {
		return false
	}
	return matchesTags(health, tags)
}

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

func (s *EndpointHealthStore) SetDraining(ctx context.Context, endpointID string, draining bool) error {
	if endpointID == "" {
		return fmt.Errorf("endpoint_id cannot be empty")
	}

	if draining {
		return s.client.Client.SAdd(ctx, endpointDrainingSetKey, endpointID).Err()
	}

	return s.client.Client.SRem(ctx, endpointDrainingSetKey, endpointID).Err()
}

func (s *EndpointHealthStore) IsDraining(ctx context.Context, endpointID string) (bool, error) {
	if endpointID == "" {
		return false, fmt.Errorf("endpoint_id cannot be empty")
	}

	return s.client.Client.SIsMember(ctx, endpointDrainingSetKey, endpointID).Result()
}

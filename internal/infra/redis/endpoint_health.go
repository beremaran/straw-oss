package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	// ErrHealthCannotBeNil is returned when UpdateHealth is called with a nil health record.
	ErrHealthCannotBeNil = errors.New("health cannot be nil")
	// ErrEmptyEndpointID is returned when an empty endpoint ID is provided.
	ErrEmptyEndpointID = errors.New("endpoint_id cannot be empty")
)

const (
	// HealthStateHealthy indicates the endpoint is operating normally.
	HealthStateHealthy = "healthy"
	// HealthStateSuspect indicates the endpoint may have issues.
	HealthStateSuspect = "suspect"
	// HealthStateUnhealthy indicates the endpoint is not functioning.
	HealthStateUnhealthy = "unhealthy"
	// HealthStateDraining indicates the endpoint is being drained of traffic.
	HealthStateDraining = "draining"
	// HealthStateDeleted indicates the endpoint has been deleted.
	HealthStateDeleted = "deleted"
)

const (
	endpointHealthKeyPrefix = "endpoint:health:"
	endpointHealthIndexKey  = "endpoint:health:index"
	endpointDrainingSetKey  = "endpoint:draining"
	endpointDeletedSetKey   = "endpoint:deleted"
)

const defaultHealthTTL = 60 * time.Second

const tagTypeResidential = "type:residential"

const tagRegionUS = "region:us"

const testEp2 = "ep-2"

// EndpointHealth represents the health status of a proxied endpoint.
type EndpointHealth struct {
	EndpointID  string    `json:"endpoint_id"`
	State       string    `json:"state"`
	Tags        []string  `json:"tags,omitempty"`
	Version     string    `json:"version,omitempty"`
	ActiveTasks int       `json:"active_tasks"`
	LastSeen    time.Time `json:"last_seen"`
}

// HealthStore manages endpoint health records in Redis.
type HealthStore interface {
	UpdateHealth(ctx context.Context, health *EndpointHealth) error

	GetHealth(ctx context.Context, endpointID string) (*EndpointHealth, error)

	ListHealthyByTags(ctx context.Context, tags []string) ([]*EndpointHealth, error)

	ListAllEndpoints(ctx context.Context) ([]*EndpointHealth, error)

	DeleteHealth(ctx context.Context, endpointID string) error

	SetDraining(ctx context.Context, endpointID string, draining bool) error

	IsDraining(ctx context.Context, endpointID string) (bool, error)

	SetDeleted(ctx context.Context, endpointID string, deleted bool) error

	IsDeleted(ctx context.Context, endpointID string) (bool, error)
}

// EndpointHealthStore implements HealthStore using Redis.
type EndpointHealthStore struct {
	client *Client
}

// NewEndpointHealthStore creates a new store backed by the given Redis client.
func NewEndpointHealthStore(client *Client) *EndpointHealthStore {
	return &EndpointHealthStore{client: client}
}

func healthKey(endpointID string) string {
	return endpointHealthKeyPrefix + endpointID
}

// UpdateHealth writes the health record and updates the sorted set index.
func (s *EndpointHealthStore) UpdateHealth(ctx context.Context, health *EndpointHealth) error {
	if health == nil {
		return ErrHealthCannotBeNil
	}

	if health.EndpointID == "" {
		return ErrEmptyEndpointID
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

// GetHealth retrieves a health record by endpoint ID.
func (s *EndpointHealthStore) GetHealth(ctx context.Context, endpointID string) (*EndpointHealth, error) {
	if endpointID == "" {
		return nil, ErrEmptyEndpointID
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

	err = json.Unmarshal(data, &health)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal health: %w", err)
	}

	return &health, nil
}

// ListHealthyByTags returns healthy or suspect endpoints matching all required tags.
func (s *EndpointHealthStore) ListHealthyByTags(ctx context.Context, tags []string) ([]*EndpointHealth, error) {
	endpoints, err := listHealthRecords(ctx, s)
	if err != nil {
		return nil, err
	}

	healthy := make([]*EndpointHealth, 0, len(endpoints))
	for _, health := range endpoints {
		if isHealthyForSelection(health) && matchesTags(health, tags) {
			healthy = append(healthy, health)
		}
	}

	return healthy, nil
}

func listHealthRecords(ctx context.Context, s *EndpointHealthStore) ([]*EndpointHealth, error) {
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

	return decodeHealthRecords(results), nil
}

// ListAllEndpoints returns all health records in the store.
func (s *EndpointHealthStore) ListAllEndpoints(ctx context.Context) ([]*EndpointHealth, error) {
	return listHealthRecords(ctx, s)
}

func decodeHealthRecords(results []any) []*EndpointHealth {
	endpoints := make([]*EndpointHealth, 0, len(results))
	for _, result := range results {
		health := decodeHealthRecord(result)
		if health != nil {
			endpoints = append(endpoints, health)
		}
	}

	return endpoints
}

func decodeHealthRecord(result any) *EndpointHealth {
	str, ok := result.(string)
	if !ok {
		return nil
	}

	health := new(EndpointHealth)

	err := json.Unmarshal([]byte(str), health)
	if err != nil {
		return nil
	}

	return health
}

func isHealthyForSelection(health *EndpointHealth) bool {
	return health.State == HealthStateHealthy || health.State == HealthStateSuspect
}

// DeleteHealth removes a health record and its index entry.
func (s *EndpointHealthStore) DeleteHealth(ctx context.Context, endpointID string) error {
	if endpointID == "" {
		return ErrEmptyEndpointID
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

// SetDraining marks or unmarks an endpoint as draining.
func (s *EndpointHealthStore) SetDraining(ctx context.Context, endpointID string, draining bool) error {
	if endpointID == "" {
		return ErrEmptyEndpointID
	}

	if draining {
		err := s.client.Client.SAdd(ctx, endpointDrainingSetKey, endpointID).Err()
		if err != nil {
			return fmt.Errorf("redis sadd draining: %w", err)
		}
	} else {
		err := s.client.Client.SRem(ctx, endpointDrainingSetKey, endpointID).Err()
		if err != nil {
			return fmt.Errorf("redis srem draining: %w", err)
		}
	}

	return nil
}

// IsDraining reports whether an endpoint is currently draining.
func (s *EndpointHealthStore) IsDraining(ctx context.Context, endpointID string) (bool, error) {
	if endpointID == "" {
		return false, ErrEmptyEndpointID
	}

	result, err := s.client.Client.SIsMember(ctx, endpointDrainingSetKey, endpointID).Result()
	if err != nil {
		return false, fmt.Errorf("redis sismember draining: %w", err)
	}

	return result, nil
}

// SetDeleted marks or unmarks an endpoint as deleted.
func (s *EndpointHealthStore) SetDeleted(ctx context.Context, endpointID string, deleted bool) error {
	if endpointID == "" {
		return ErrEmptyEndpointID
	}

	if deleted {
		err := s.client.Client.SAdd(ctx, endpointDeletedSetKey, endpointID).Err()
		if err != nil {
			return fmt.Errorf("redis sadd deleted: %w", err)
		}
	} else {
		err := s.client.Client.SRem(ctx, endpointDeletedSetKey, endpointID).Err()
		if err != nil {
			return fmt.Errorf("redis srem deleted: %w", err)
		}
	}

	return nil
}

// IsDeleted reports whether an endpoint is marked as deleted.
func (s *EndpointHealthStore) IsDeleted(ctx context.Context, endpointID string) (bool, error) {
	if endpointID == "" {
		return false, ErrEmptyEndpointID
	}

	result, err := s.client.Client.SIsMember(ctx, endpointDeletedSetKey, endpointID).Result()
	if err != nil {
		return false, fmt.Errorf("redis sismember deleted: %w", err)
	}

	return result, nil
}

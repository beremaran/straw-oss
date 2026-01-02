package redis

import (
	"context"
	"testing"
	"time"
)

func TestEndpointHealthStore_UpdateAndGetHealth(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	store := NewEndpointHealthStore(client)
	ctx := context.Background()

	health := &EndpointHealth{
		EndpointID:  "test-endpoint-1",
		State:       HealthStateHealthy,
		Tags:        []string{"type:residential", "region:us"},
		Version:     "1.0.0",
		ActiveTasks: 5,
		LastSeen:    time.Now(),
	}

	// Update health
	err := store.UpdateHealth(ctx, health)
	if err != nil {
		t.Fatalf("UpdateHealth failed: %v", err)
	}

	// Get health
	retrieved, err := store.GetHealth(ctx, "test-endpoint-1")
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}

	if retrieved.EndpointID != health.EndpointID {
		t.Errorf("expected endpoint_id %s, got %s", health.EndpointID, retrieved.EndpointID)
	}
	if retrieved.State != health.State {
		t.Errorf("expected state %s, got %s", health.State, retrieved.State)
	}
	if retrieved.Version != health.Version {
		t.Errorf("expected version %s, got %s", health.Version, retrieved.Version)
	}
	if retrieved.ActiveTasks != health.ActiveTasks {
		t.Errorf("expected active_tasks %d, got %d", health.ActiveTasks, retrieved.ActiveTasks)
	}
}

func TestEndpointHealthStore_GetHealth_NotFound(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	store := NewEndpointHealthStore(client)
	ctx := context.Background()

	_, err := store.GetHealth(ctx, "nonexistent-endpoint")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
}

func TestEndpointHealthStore_ListHealthyByTags(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	store := NewEndpointHealthStore(client)
	ctx := context.Background()

	// Create test endpoints
	endpoints := []*EndpointHealth{
		{
			EndpointID: "ep-1",
			State:      HealthStateHealthy,
			Tags:       []string{"type:residential", "region:us"},
			LastSeen:   time.Now(),
		},
		{
			EndpointID: "ep-2",
			State:      HealthStateHealthy,
			Tags:       []string{"type:residential", "region:eu"},
			LastSeen:   time.Now(),
		},
		{
			EndpointID: "ep-3",
			State:      HealthStateUnhealthy,
			Tags:       []string{"type:residential", "region:us"},
			LastSeen:   time.Now(),
		},
		{
			EndpointID: "ep-4",
			State:      HealthStateSuspect,
			Tags:       []string{"type:datacenter", "region:us"},
			LastSeen:   time.Now(),
		},
	}

	for _, ep := range endpoints {
		if err := store.UpdateHealth(ctx, ep); err != nil {
			t.Fatalf("UpdateHealth failed for %s: %v", ep.EndpointID, err)
		}
	}

	tests := []struct {
		name     string
		tags     []string
		expected int
	}{
		{
			name:     "filter by type:residential",
			tags:     []string{"type:residential"},
			expected: 2, // ep-1, ep-2 (ep-3 is unhealthy)
		},
		{
			name:     "filter by region:us",
			tags:     []string{"region:us"},
			expected: 2, // ep-1, ep-4 (ep-3 is unhealthy)
		},
		{
			name:     "filter by residential AND us",
			tags:     []string{"type:residential", "region:us"},
			expected: 1, // ep-1 only
		},
		{
			name:     "no filter (all healthy/suspect)",
			tags:     []string{},
			expected: 3, // ep-1, ep-2, ep-4
		},
		{
			name:     "filter by non-existent tag",
			tags:     []string{"type:mobile"},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthy, err := store.ListHealthyByTags(ctx, tt.tags)
			if err != nil {
				t.Fatalf("ListHealthyByTags failed: %v", err)
			}
			if len(healthy) != tt.expected {
				t.Errorf("expected %d endpoints, got %d", tt.expected, len(healthy))
			}
		})
	}
}

func TestEndpointHealthStore_ListAllEndpoints(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	store := NewEndpointHealthStore(client)
	ctx := context.Background()

	// Create test endpoints
	endpoints := []*EndpointHealth{
		{EndpointID: "ep-1", State: HealthStateHealthy, LastSeen: time.Now()},
		{EndpointID: "ep-2", State: HealthStateUnhealthy, LastSeen: time.Now()},
		{EndpointID: "ep-3", State: HealthStateSuspect, LastSeen: time.Now()},
	}

	for _, ep := range endpoints {
		if err := store.UpdateHealth(ctx, ep); err != nil {
			t.Fatalf("UpdateHealth failed for %s: %v", ep.EndpointID, err)
		}
	}

	all, err := store.ListAllEndpoints(ctx)
	if err != nil {
		t.Fatalf("ListAllEndpoints failed: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("expected 3 endpoints, got %d", len(all))
	}
}

func TestEndpointHealthStore_DeleteHealth(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	store := NewEndpointHealthStore(client)
	ctx := context.Background()

	// Create endpoint
	health := &EndpointHealth{
		EndpointID: "ep-to-delete",
		State:      HealthStateHealthy,
		LastSeen:   time.Now(),
	}

	if err := store.UpdateHealth(ctx, health); err != nil {
		t.Fatalf("UpdateHealth failed: %v", err)
	}

	// Verify it exists
	_, err := store.GetHealth(ctx, "ep-to-delete")
	if err != nil {
		t.Fatalf("GetHealth failed: %v", err)
	}

	// Delete it
	if err := store.DeleteHealth(ctx, "ep-to-delete"); err != nil {
		t.Fatalf("DeleteHealth failed: %v", err)
	}

	// Verify it's gone
	_, err = store.GetHealth(ctx, "ep-to-delete")
	if err != ErrCacheMiss {
		t.Errorf("expected ErrCacheMiss after delete, got %v", err)
	}

	// Verify it's removed from index
	all, err := store.ListAllEndpoints(ctx)
	if err != nil {
		t.Fatalf("ListAllEndpoints failed: %v", err)
	}
	for _, ep := range all {
		if ep.EndpointID == "ep-to-delete" {
			t.Error("deleted endpoint still in index")
		}
	}
}

func TestEndpointHealthStore_Validation(t *testing.T) {
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	store := NewEndpointHealthStore(client)
	ctx := context.Background()

	// Test nil health
	err := store.UpdateHealth(ctx, nil)
	if err == nil {
		t.Error("expected error for nil health")
	}

	// Test empty endpoint ID
	err = store.UpdateHealth(ctx, &EndpointHealth{State: HealthStateHealthy})
	if err == nil {
		t.Error("expected error for empty endpoint_id")
	}

	// Test empty endpoint ID for GetHealth
	_, err = store.GetHealth(ctx, "")
	if err == nil {
		t.Error("expected error for empty endpoint_id in GetHealth")
	}
}

func TestMatchesTags(t *testing.T) {
	tests := []struct {
		name         string
		healthTags   []string
		requiredTags []string
		expected     bool
	}{
		{
			name:         "no required tags",
			healthTags:   []string{"a", "b"},
			requiredTags: []string{},
			expected:     true,
		},
		{
			name:         "all required tags present",
			healthTags:   []string{"a", "b", "c"},
			requiredTags: []string{"a", "b"},
			expected:     true,
		},
		{
			name:         "missing required tag",
			healthTags:   []string{"a", "c"},
			requiredTags: []string{"a", "b"},
			expected:     false,
		},
		{
			name:         "exact match",
			healthTags:   []string{"a", "b"},
			requiredTags: []string{"a", "b"},
			expected:     true,
		},
		{
			name:         "empty health tags",
			healthTags:   []string{},
			requiredTags: []string{"a"},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			health := &EndpointHealth{Tags: tt.healthTags}
			result := matchesTags(health, tt.requiredTags)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// setupTestRedis creates a test Redis client.
// In a real test environment, this would connect to a test Redis instance.
// For now, we'll skip tests if Redis is not available.
func setupTestRedis(t *testing.T) (*Client, func()) {
	t.Helper()

	client, err := NewClient("localhost:6379", nil)
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// Use a unique prefix for test isolation
	cleanup := func() {
		ctx := context.Background()
		// Clean up test keys
		keys, _ := client.Client.Keys(ctx, "endpoint:health:*").Result()
		if len(keys) > 0 {
			client.Client.Del(ctx, keys...)
		}
		client.Close()
	}

	// Clean up any existing test data
	ctx := context.Background()
	keys, _ := client.Client.Keys(ctx, "endpoint:health:*").Result()
	if len(keys) > 0 {
		client.Client.Del(ctx, keys...)
	}

	return client, cleanup
}

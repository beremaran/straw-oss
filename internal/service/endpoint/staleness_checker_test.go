package endpoint

import (
	"context"
	"testing"
	"time"

	"github.com/kwilabs/straw-proxy-server/internal/infra/redis"
)

func TestStalenessChecker_DetermineState(t *testing.T) {
	checker := &StalenessChecker{
		suspectThreshold:   15 * time.Second,
		unhealthyThreshold: 30 * time.Second,
	}

	tests := []struct {
		name          string
		staleDuration time.Duration
		currentState  string
		expected      string
	}{
		{
			name:          "fresh heartbeat - stays healthy",
			staleDuration: 5 * time.Second,
			currentState:  redis.HealthStateHealthy,
			expected:      redis.HealthStateHealthy,
		},
		{
			name:          "missed 1 heartbeat - becomes suspect",
			staleDuration: 16 * time.Second,
			currentState:  redis.HealthStateHealthy,
			expected:      redis.HealthStateSuspect,
		},
		{
			name:          "missed 3 heartbeats - becomes unhealthy",
			staleDuration: 31 * time.Second,
			currentState:  redis.HealthStateHealthy,
			expected:      redis.HealthStateUnhealthy,
		},
		{
			name:          "already suspect but not stale enough for unhealthy",
			staleDuration: 20 * time.Second,
			currentState:  redis.HealthStateSuspect,
			expected:      redis.HealthStateSuspect,
		},
		{
			name:          "suspect becomes unhealthy",
			staleDuration: 35 * time.Second,
			currentState:  redis.HealthStateSuspect,
			expected:      redis.HealthStateUnhealthy,
		},
		{
			name:          "fresh heartbeat on suspect - becomes healthy",
			staleDuration: 5 * time.Second,
			currentState:  redis.HealthStateSuspect,
			expected:      redis.HealthStateHealthy,
		},
		{
			name:          "fresh heartbeat on unhealthy - becomes healthy",
			staleDuration: 5 * time.Second,
			currentState:  redis.HealthStateUnhealthy,
			expected:      redis.HealthStateHealthy,
		},
		{
			name:          "unhealthy stays unhealthy when stale",
			staleDuration: 40 * time.Second,
			currentState:  redis.HealthStateUnhealthy,
			expected:      redis.HealthStateUnhealthy,
		},
		{
			name:          "boundary: exactly at suspect threshold",
			staleDuration: 15 * time.Second,
			currentState:  redis.HealthStateHealthy,
			expected:      redis.HealthStateSuspect,
		},
		{
			name:          "boundary: exactly at unhealthy threshold",
			staleDuration: 30 * time.Second,
			currentState:  redis.HealthStateHealthy,
			expected:      redis.HealthStateUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.determineState(tt.staleDuration, tt.currentState)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestStalenessChecker_CheckStaleness(t *testing.T) {
	store := newMockHealthStore()
	checker := NewStalenessChecker(store,
		WithSuspectThreshold(15*time.Second),
		WithUnhealthyThreshold(30*time.Second),
	)

	ctx := context.Background()
	now := time.Now()

	// Create test endpoints with different staleness levels
	store.endpoints["fresh"] = &redis.EndpointHealth{
		EndpointID: "fresh",
		State:      redis.HealthStateHealthy,
		LastSeen:   now.Add(-5 * time.Second), // 5s ago - fresh
	}
	store.endpoints["suspect-candidate"] = &redis.EndpointHealth{
		EndpointID: "suspect-candidate",
		State:      redis.HealthStateHealthy,
		LastSeen:   now.Add(-20 * time.Second), // 20s ago - should become suspect
	}
	store.endpoints["unhealthy-candidate"] = &redis.EndpointHealth{
		EndpointID: "unhealthy-candidate",
		State:      redis.HealthStateSuspect,
		LastSeen:   now.Add(-35 * time.Second), // 35s ago - should become unhealthy
	}

	// Run staleness check
	checker.checkStaleness(ctx)

	// Verify state transitions
	if store.endpoints["fresh"].State != redis.HealthStateHealthy {
		t.Errorf("fresh endpoint: expected healthy, got %s", store.endpoints["fresh"].State)
	}
	if store.endpoints["suspect-candidate"].State != redis.HealthStateSuspect {
		t.Errorf("suspect-candidate endpoint: expected suspect, got %s", store.endpoints["suspect-candidate"].State)
	}
	if store.endpoints["unhealthy-candidate"].State != redis.HealthStateUnhealthy {
		t.Errorf("unhealthy-candidate endpoint: expected unhealthy, got %s", store.endpoints["unhealthy-candidate"].State)
	}
}

func TestStalenessChecker_StartStop(t *testing.T) {
	store := newMockHealthStore()
	checker := NewStalenessChecker(store,
		WithCheckInterval(50*time.Millisecond),
	)

	ctx := context.Background()

	// Start the checker
	checker.Start(ctx)

	if !checker.IsRunning() {
		t.Error("checker should be running after Start")
	}

	// Wait for a few check cycles
	time.Sleep(150 * time.Millisecond)

	// Stop the checker
	checker.Stop()

	if checker.IsRunning() {
		t.Error("checker should not be running after Stop")
	}
}

func TestStalenessChecker_StartWhileRunning(t *testing.T) {
	store := newMockHealthStore()
	checker := NewStalenessChecker(store,
		WithCheckInterval(100*time.Millisecond),
	)

	ctx := context.Background()

	checker.Start(ctx)
	defer checker.Stop()

	// Try to start again - should be a no-op
	checker.Start(ctx)

	if !checker.IsRunning() {
		t.Error("checker should still be running")
	}
}

func TestStalenessChecker_StopWhileNotRunning(t *testing.T) {
	store := newMockHealthStore()
	checker := NewStalenessChecker(store)

	// Stop without starting - should be a no-op (no panic)
	checker.Stop()

	if checker.IsRunning() {
		t.Error("checker should not be running")
	}
}

func TestStalenessChecker_CustomThresholds(t *testing.T) {
	store := newMockHealthStore()
	checker := NewStalenessChecker(store,
		WithCheckInterval(10*time.Millisecond),
		WithSuspectThreshold(5*time.Second),
		WithUnhealthyThreshold(10*time.Second),
	)

	if checker.interval != 10*time.Millisecond {
		t.Errorf("expected interval 10ms, got %v", checker.interval)
	}
	if checker.suspectThreshold != 5*time.Second {
		t.Errorf("expected suspect threshold 5s, got %v", checker.suspectThreshold)
	}
	if checker.unhealthyThreshold != 10*time.Second {
		t.Errorf("expected unhealthy threshold 10s, got %v", checker.unhealthyThreshold)
	}
}

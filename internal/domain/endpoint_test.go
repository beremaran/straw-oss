package domain

import (
	"testing"
	"time"
)

func TestEndpoint_IsStale(t *testing.T) {
	tests := []struct {
		name          string
		lastHeartbeat time.Time
		threshold     time.Duration
		want          bool
	}{
		{
			name:          "healthy",
			lastHeartbeat: time.Now().Add(-10 * time.Second),
			threshold:     30 * time.Second,
			want:          false,
		},
		{
			name:          "stale",
			lastHeartbeat: time.Now().Add(-60 * time.Second),
			threshold:     30 * time.Second,
			want:          true,
		},
		{
			name:          "just beat",
			lastHeartbeat: time.Now(),
			threshold:     30 * time.Second,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Endpoint{LastHeartbeat: tt.lastHeartbeat}
			if got := e.IsStale(tt.threshold); got != tt.want {
				t.Errorf("Endpoint.IsStale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpoint_UpdateHeartbeat(t *testing.T) {
	e := &Endpoint{
		LastHeartbeat: time.Now().Add(-1 * time.Hour),
		IsHealthy:     false,
	}

	e.UpdateHeartbeat()

	if !e.IsHealthy {
		t.Error("UpdateHeartbeat() did not set IsHealthy to true")
	}

	if time.Since(e.LastHeartbeat) > time.Second {
		t.Error("UpdateHeartbeat() did not update LastHeartbeat")
	}
}

func TestEndpoint_HasTag(t *testing.T) {
	e := &Endpoint{
		Tags: []string{"type:residential", "region:us", "provider:luminati"},
	}

	if !e.HasTag("type:residential") {
		t.Error("HasTag() returned false for existing tag")
	}

	if e.HasTag("type:datacenter") {
		t.Error("HasTag() returned true for non-existing tag")
	}
}

func TestEndpoint_MatchesTags(t *testing.T) {
	e := &Endpoint{
		Tags: []string{"type:residential", "region:us", "provider:luminati"},
	}

	tests := []struct {
		name         string
		requiredTags []string
		want         bool
	}{
		{
			name:         "all match",
			requiredTags: []string{"type:residential", "region:us"},
			want:         true,
		},
		{
			name:         "one missing",
			requiredTags: []string{"type:residential", "region:eu"},
			want:         false,
		},
		{
			name:         "empty required",
			requiredTags: []string{},
			want:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.MatchesTags(tt.requiredTags); got != tt.want {
				t.Errorf("Endpoint.MatchesTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewEndpoint(t *testing.T) {
	e := NewEndpoint("ep-001", []string{"type:residential", "region:us"})

	if e.ID != "ep-001" {
		t.Errorf("NewEndpoint() ID = %s, want ep-001", e.ID)
	}
	if len(e.Tags) != 2 {
		t.Errorf("NewEndpoint() Tags length = %d, want 2", len(e.Tags))
	}
	if !e.IsHealthy {
		t.Error("NewEndpoint() IsHealthy = false, want true")
	}
}

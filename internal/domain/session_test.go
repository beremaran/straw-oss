package domain

import (
	"testing"
	"time"
)

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name       string
		lastUsedAt time.Time
		ttl        time.Duration
		want       bool
	}{
		{
			name:       "not expired",
			lastUsedAt: time.Now().Add(-5 * time.Minute),
			ttl:        10 * time.Minute,
			want:       false,
		},
		{
			name:       "expired",
			lastUsedAt: time.Now().Add(-15 * time.Minute),
			ttl:        10 * time.Minute,
			want:       true,
		},
		{
			name:       "just created",
			lastUsedAt: time.Now(),
			ttl:        10 * time.Minute,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{LastUsedAt: tt.lastUsedAt}
			if got := s.IsExpired(tt.ttl); got != tt.want {
				t.Errorf("Session.IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSession_Touch(t *testing.T) {
	s := &Session{
		RequestCount: 5,
		LastUsedAt:   time.Now().Add(-5 * time.Minute),
	}

	before := s.RequestCount
	s.Touch()

	if s.RequestCount != before+1 {
		t.Errorf("Touch() did not increment RequestCount, got %d, want %d", s.RequestCount, before+1)
	}

	if time.Since(s.LastUsedAt) > time.Second {
		t.Errorf("Touch() did not update LastUsedAt")
	}
}

func TestSession_CanMigrate(t *testing.T) {
	tests := []struct {
		name           string
		migrationCount int
		want           bool
	}{
		{name: "can migrate", migrationCount: 0, want: true},
		{name: "at limit", migrationCount: 3, want: false},
		{name: "over limit", migrationCount: 5, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{MigrationCount: tt.migrationCount}
			if got := s.CanMigrate(); got != tt.want {
				t.Errorf("Session.CanMigrate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSession_Migrate(t *testing.T) {
	s := &Session{
		EndpointID:     "old-endpoint",
		MigrationCount: 0,
	}

	if !s.Migrate("new-endpoint") {
		t.Error("Migrate() returned false, expected true")
	}

	if s.EndpointID != "new-endpoint" {
		t.Errorf("Migrate() did not update EndpointID, got %s", s.EndpointID)
	}

	if s.MigrationCount != 1 {
		t.Errorf("Migrate() did not increment MigrationCount, got %d", s.MigrationCount)
	}

	s.MigrationCount = MaxMigrationCount
	if s.Migrate("another-endpoint") {
		t.Error("Migrate() returned true when at limit")
	}
}

func TestNewSession(t *testing.T) {
	s := NewSession("sess-123", "ep-001", "rule-001", []string{targetAmazon})

	if s.ID != "sess-123" {
		t.Errorf("NewSession() ID = %s, want sess-123", s.ID)
	}
	if s.EndpointID != "ep-001" {
		t.Errorf("NewSession() EndpointID = %s, want ep-001", s.EndpointID)
	}
	if s.MigrationCount != 0 {
		t.Errorf("NewSession() MigrationCount = %d, want 0", s.MigrationCount)
	}
	if s.RequestCount != 0 {
		t.Errorf("NewSession() RequestCount = %d, want 0", s.RequestCount)
	}
}

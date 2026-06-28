package domain_test

import (
	"testing"
	"time"

	"github.com/beremaran/straw/internal/domain"
)

func TestApiKey_IsValid(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name   string
		apiKey domain.ApiKey
		want   bool
	}{
		{
			name:   "active and not expired",
			apiKey: domain.ApiKey{IsActive: true, ExpiresAt: &future},
			want:   true,
		},
		{
			name:   "active with no expiration",
			apiKey: domain.ApiKey{IsActive: true, ExpiresAt: nil},
			want:   true,
		},
		{
			name:   "inactive",
			apiKey: domain.ApiKey{IsActive: false, ExpiresAt: &future},
			want:   false,
		},
		{
			name:   "expired",
			apiKey: domain.ApiKey{IsActive: true, ExpiresAt: &past},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.apiKey.IsValid(); got != tt.want {
				t.Errorf("ApiKey.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApiKey_HasScope(t *testing.T) {
	apiKey := &domain.ApiKey{
		Scopes: []string{"target:*", "type:search", "region:us"},
	}

	tests := []struct {
		name string
		tag  domain.Tag
		want bool
	}{
		{
			name: "wildcard match",
			tag:  domain.Tag{Key: "target", Value: "amazon"},
			want: true,
		},
		{
			name: "exact match",
			tag:  domain.Tag{Key: "type", Value: "search"},
			want: true,
		},
		{
			name: "no match - different value",
			tag:  domain.Tag{Key: "type", Value: "product"},
			want: false,
		},
		{
			name: "no match - different key",
			tag:  domain.Tag{Key: "capability", Value: "stealth"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apiKey.HasScope(tt.tag); got != tt.want {
				t.Errorf("ApiKey.HasScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApiKey_HasScopeForTags(t *testing.T) {
	apiKey := &domain.ApiKey{
		Scopes: []string{"target:*", "type:search"},
	}

	tests := []struct {
		name string
		tags []domain.Tag
		want bool
	}{
		{
			name: "all tags covered",
			tags: []domain.Tag{{Key: "target", Value: "amazon"}, {Key: "type", Value: "search"}},
			want: true,
		},
		{
			name: "one tag not covered",
			tags: []domain.Tag{{Key: "target", Value: "amazon"}, {Key: "region", Value: "us"}},
			want: false,
		},
		{
			name: "empty tags",
			tags: []domain.Tag{},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apiKey.HasScopeForTags(tt.tags); got != tt.want {
				t.Errorf("ApiKey.HasScopeForTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewApiKey(t *testing.T) {
	key := domain.NewApiKey("key-123", "hashed", "Test Key", []string{"target:*"})

	if key.ID != "key-123" {
		t.Errorf("NewApiKey() ID = %s, want key-123", key.ID)
	}
	if !key.IsActive {
		t.Error("NewApiKey() IsActive = false, want true")
	}
	if key.ExpiresAt != nil {
		t.Error("NewApiKey() ExpiresAt should be nil")
	}
}

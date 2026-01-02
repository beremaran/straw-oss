package domain

import (
	"testing"
	"time"
)

func TestApiKey_IsValid(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name   string
		apiKey ApiKey
		want   bool
	}{
		{
			name:   "active and not expired",
			apiKey: ApiKey{IsActive: true, ExpiresAt: &future},
			want:   true,
		},
		{
			name:   "active with no expiration",
			apiKey: ApiKey{IsActive: true, ExpiresAt: nil},
			want:   true,
		},
		{
			name:   "inactive",
			apiKey: ApiKey{IsActive: false, ExpiresAt: &future},
			want:   false,
		},
		{
			name:   "expired",
			apiKey: ApiKey{IsActive: true, ExpiresAt: &past},
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
	apiKey := &ApiKey{
		Scopes: []string{"target:*", "type:search", "region:us"},
	}

	tests := []struct {
		name string
		tag  Tag
		want bool
	}{
		{
			name: "wildcard match",
			tag:  Tag{Key: "target", Value: "amazon"},
			want: true,
		},
		{
			name: "exact match",
			tag:  Tag{Key: "type", Value: "search"},
			want: true,
		},
		{
			name: "no match - different value",
			tag:  Tag{Key: "type", Value: "product"},
			want: false,
		},
		{
			name: "no match - different key",
			tag:  Tag{Key: "capability", Value: "stealth"},
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
	apiKey := &ApiKey{
		Scopes: []string{"target:*", "type:search"},
	}

	tests := []struct {
		name string
		tags []Tag
		want bool
	}{
		{
			name: "all tags covered",
			tags: []Tag{{Key: "target", Value: "amazon"}, {Key: "type", Value: "search"}},
			want: true,
		},
		{
			name: "one tag not covered",
			tags: []Tag{{Key: "target", Value: "amazon"}, {Key: "region", Value: "us"}},
			want: false,
		},
		{
			name: "empty tags",
			tags: []Tag{},
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

func TestMatchScope(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		tag   string
		want  bool
	}{
		{name: "exact match", scope: "target:amazon", tag: "target:amazon", want: true},
		{name: "wildcard suffix", scope: "target:*", tag: "target:amazon", want: true},
		{name: "wildcard prefix", scope: "*:search", tag: "type:search", want: true},
		{name: "full wildcard", scope: "*", tag: "anything:here", want: true},
		{name: "no match", scope: "target:walmart", tag: "target:amazon", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchScope(tt.scope, tt.tag); got != tt.want {
				t.Errorf("matchScope(%q, %q) = %v, want %v", tt.scope, tt.tag, got, tt.want)
			}
		})
	}
}

func TestNewApiKey(t *testing.T) {
	key := NewApiKey("key-123", "hashed", "Test Key", []string{"target:*"})

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

package domain

import (
	"testing"
	"time"
)

const regionUS = "region:us"

const targetWildcard = "target:*"

const typeSearch = "type:search"

const amazon = "amazon"

const target = "target"

const exactMatch = "exact match"

const search = "search"

const tagType = "type"

const region = "region"

const targetAmazon = "target:amazon"

func TestApiKey_IsValid(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)

	tests := []struct {
		name   string
		apiKey APIKey
		want   bool
	}{
		{
			name:   "active and not expired",
			apiKey: APIKey{IsActive: true, ExpiresAt: &future},
			want:   true,
		},
		{
			name:   "active with no expiration",
			apiKey: APIKey{IsActive: true, ExpiresAt: nil},
			want:   true,
		},
		{
			name:   "inactive",
			apiKey: APIKey{IsActive: false, ExpiresAt: &future},
			want:   false,
		},
		{
			name:   "expired",
			apiKey: APIKey{IsActive: true, ExpiresAt: &past},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.apiKey.IsValid(); got != tt.want {
				t.Errorf("APIKey.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApiKey_HasScope(t *testing.T) {
	apiKey := &APIKey{
		Scopes: []string{targetWildcard, typeSearch, regionUS},
	}

	tests := []struct {
		name string
		tag  Tag
		want bool
	}{
		{
			name: "wildcard match",
			tag:  Tag{Key: target, Value: amazon},
			want: true,
		},
		{
			name: exactMatch,
			tag:  Tag{Key: tagType, Value: search},
			want: true,
		},
		{
			name: "no match - different value",
			tag:  Tag{Key: tagType, Value: "product"},
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
				t.Errorf("APIKey.HasScope() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApiKey_HasScopeForTags(t *testing.T) {
	apiKey := &APIKey{
		Scopes: []string{targetWildcard, typeSearch},
	}

	tests := []struct {
		name string
		tags []Tag
		want bool
	}{
		{
			name: "all tags covered",
			tags: []Tag{{Key: target, Value: amazon}, {Key: tagType, Value: search}},
			want: true,
		},
		{
			name: "one tag not covered",
			tags: []Tag{{Key: target, Value: amazon}, {Key: region, Value: "us"}},
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
				t.Errorf("APIKey.HasScopeForTags() = %v, want %v", got, tt.want)
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
		{name: exactMatch, scope: targetAmazon, tag: targetAmazon, want: true},
		{name: "wildcard suffix", scope: targetWildcard, tag: targetAmazon, want: true},
		{name: "wildcard prefix", scope: "*:search", tag: typeSearch, want: true},
		{name: "full wildcard", scope: "*", tag: "anything:here", want: true},
		{name: "no match", scope: "target:walmart", tag: targetAmazon, want: false},
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
	key := NewAPIKey("key-123", "hashed", "Test Key", []string{targetWildcard})

	if key.ID != "key-123" {
		t.Errorf("NewAPIKey() ID = %s, want key-123", key.ID)
	}
	if !key.IsActive {
		t.Error("NewAPIKey() IsActive = false, want true")
	}
	if key.ExpiresAt != nil {
		t.Error("NewAPIKey() ExpiresAt should be nil")
	}
}

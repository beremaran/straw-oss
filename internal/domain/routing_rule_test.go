package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRoutingRule_JSON(t *testing.T) {
	rule := RoutingRule{
		ID:                   "rule-001",
		Name:                 "Amazon Search",
		RequiredTags:         []string{targetAmazon, "type:search"},
		ExcludedTags:         []string{"region:eu"},
		Priority:             100,
		HardTimeout:          30 * time.Second,
		RateLimitPerMinute:   60,
		FingerprintPreset:    "chrome-130",
		AllowedEndpointTypes: []string{"residential", "mobile"},
		IsActive:             true,
		Version:              1,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	data, err := json.Marshal(rule)
	if err != nil {
		t.Fatalf("Failed to marshal RoutingRule: %v", err)
	}

	var decoded RoutingRule
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal RoutingRule: %v", err)
	}

	if decoded.ID != rule.ID {
		t.Errorf("Decoded ID = %s, want %s", decoded.ID, rule.ID)
	}
	if decoded.Name != rule.Name {
		t.Errorf("Decoded Name = %s, want %s", decoded.Name, rule.Name)
	}
	if len(decoded.RequiredTags) != len(rule.RequiredTags) {
		t.Errorf("Decoded RequiredTags length = %d, want %d", len(decoded.RequiredTags), len(rule.RequiredTags))
	}
}

func TestRoutingRule_MatchesTags(t *testing.T) {
	rule := &RoutingRule{
		RequiredTags: []string{targetAmazon, "type:search"},
		ExcludedTags: []string{"region:eu"},
	}

	tests := []struct {
		name string
		tags []Tag
		want bool
	}{
		{
			name: "matches all required, no excluded",
			tags: []Tag{
				{Key: target, Value: amazon},
				{Key: tagType, Value: search},
				{Key: region, Value: "us"},
			},
			want: true,
		},
		{
			name: "missing required tag",
			tags: []Tag{
				{Key: target, Value: amazon},
			},
			want: false,
		},
		{
			name: "has excluded tag",
			tags: []Tag{
				{Key: target, Value: amazon},
				{Key: tagType, Value: search},
				{Key: region, Value: "eu"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rule.MatchesTags(tt.tags); got != tt.want {
				t.Errorf("RoutingRule.MatchesTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestABConfig_JSON(t *testing.T) {
	config := ABConfig{
		Variants: []ABVariant{
			{Fingerprint: "chrome-130", Weight: 60},
			{Fingerprint: "firefox-133", Weight: 40},
		},
		Strategy: "weighted",
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal ABConfig: %v", err)
	}

	var decoded ABConfig
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal ABConfig: %v", err)
	}

	if decoded.Strategy != config.Strategy {
		t.Errorf("Decoded Strategy = %s, want %s", decoded.Strategy, config.Strategy)
	}
	if len(decoded.Variants) != len(config.Variants) {
		t.Errorf("Decoded Variants length = %d, want %d", len(decoded.Variants), len(config.Variants))
	}
}

func TestRequestFilter_JSON(t *testing.T) {
	filter := RequestFilter{
		BlockContentTypes: []string{"image/*", "font/*"},
		BlockURLPatterns:  []string{"*.google-analytics.com"},
		BlockDomains:      []string{"ads.example.com"},
		EnableAdblock:     true,
		AdblockLists:      []string{"easylist", "easyprivacy"},
	}

	data, err := json.Marshal(filter)
	if err != nil {
		t.Fatalf("Failed to marshal RequestFilter: %v", err)
	}

	var decoded RequestFilter
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal RequestFilter: %v", err)
	}

	if decoded.EnableAdblock != filter.EnableAdblock {
		t.Errorf("Decoded EnableAdblock = %v, want %v", decoded.EnableAdblock, filter.EnableAdblock)
	}
}

func TestEndpointPool_JSON(t *testing.T) {
	pool := EndpointPool{
		Tier:       1,
		Endpoints:  []string{"endpoint-001", "endpoint-002"},
		MaxRetries: 3,
	}

	data, err := json.Marshal(pool)
	if err != nil {
		t.Fatalf("Failed to marshal EndpointPool: %v", err)
	}

	var decoded EndpointPool
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Failed to unmarshal EndpointPool: %v", err)
	}

	if decoded.Tier != pool.Tier {
		t.Errorf("Decoded Tier = %d, want %d", decoded.Tier, pool.Tier)
	}
}

package domain

import (
	"testing"
)

func TestParseTag(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Tag
		wantErr bool
	}{
		{
			name:  "valid tag",
			input: targetAmazon,
			want:  Tag{Key: target, Value: amazon},
		},
		{
			name:  "tag with whitespace",
			input: "  target : amazon  ",
			want:  Tag{Key: target, Value: amazon},
		},
		{
			name:  "tag with empty value",
			input: "target:",
			want:  Tag{Key: target, Value: ""},
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "no colon separator",
			input:   "targetamazon",
			wantErr: true,
		},
		{
			name:    "empty key",
			input:   ":amazon",
			wantErr: true,
		},
		{
			name:  "value with colons",
			input: "url:http://example.com",
			want:  Tag{Key: "url", Value: "http://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTag(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTag() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !tt.wantErr && (got.Key != tt.want.Key || got.Value != tt.want.Value) {
				t.Errorf("ParseTag() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Tag
		wantErr bool
	}{
		{
			name:  "multiple tags",
			input: "target:amazon, type:search",
			want: []Tag{
				{Key: target, Value: amazon},
				{Key: tagType, Value: search},
			},
		},
		{
			name:  "single tag",
			input: targetAmazon,
			want:  []Tag{{Key: target, Value: amazon}},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:    "invalid tag in list",
			input:   "target:amazon, invalidtag",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTags(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTags() error = %v, wantErr %v", err, tt.wantErr)

				return
			}
			if !tt.wantErr && len(got) != len(tt.want) {
				t.Errorf("ParseTags() returned %d tags, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestTagString(t *testing.T) {
	tag := Tag{Key: target, Value: amazon}
	if got := tag.String(); got != targetAmazon {
		t.Errorf("Tag.String() = %v, want %v", got, targetAmazon)
	}
}

func TestTagMatches(t *testing.T) {
	tests := []struct {
		name    string
		tag     Tag
		pattern Tag
		want    bool
	}{
		{
			name:    "exact match",
			tag:     Tag{Key: target, Value: amazon},
			pattern: Tag{Key: target, Value: amazon},
			want:    true,
		},
		{
			name:    "wildcard match",
			tag:     Tag{Key: target, Value: amazon},
			pattern: Tag{Key: target, Value: "*"},
			want:    true,
		},
		{
			name:    "different key",
			tag:     Tag{Key: target, Value: amazon},
			pattern: Tag{Key: tagType, Value: amazon},
			want:    false,
		},
		{
			name:    "different value",
			tag:     Tag{Key: target, Value: amazon},
			pattern: Tag{Key: target, Value: "walmart"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tag.Matches(tt.pattern); got != tt.want {
				t.Errorf("Tag.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesAll(t *testing.T) {
	tags := []Tag{
		{Key: target, Value: amazon},
		{Key: tagType, Value: search},
		{Key: "region", Value: "us"},
	}

	tests := []struct {
		name     string
		required []Tag
		want     bool
	}{
		{
			name:     "all required present",
			required: []Tag{{Key: target, Value: amazon}, {Key: tagType, Value: search}},
			want:     true,
		},
		{
			name:     "one required missing",
			required: []Tag{{Key: target, Value: "walmart"}},
			want:     false,
		},
		{
			name:     "empty required",
			required: []Tag{},
			want:     true,
		},
		{
			name:     "wildcard required",
			required: []Tag{{Key: target, Value: "*"}},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesAll(tags, tt.required); got != tt.want {
				t.Errorf("MatchesAll() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesNone(t *testing.T) {
	tags := []Tag{
		{Key: target, Value: amazon},
		{Key: tagType, Value: search},
	}

	tests := []struct {
		name     string
		excluded []Tag
		want     bool
	}{
		{
			name:     "no excluded present",
			excluded: []Tag{{Key: "region", Value: "eu"}},
			want:     true,
		},
		{
			name:     "excluded present",
			excluded: []Tag{{Key: target, Value: amazon}},
			want:     false,
		},
		{
			name:     "empty excluded",
			excluded: []Tag{},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesNone(tags, tt.excluded); got != tt.want {
				t.Errorf("MatchesNone() = %v, want %v", got, tt.want)
			}
		})
	}
}

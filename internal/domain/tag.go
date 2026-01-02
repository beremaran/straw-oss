// Package domain contains core business models for the Straw Proxy system.
package domain

import (
	"fmt"
	"strings"
)

// Tag represents a key:value identifier used for routing and matching.
// Tags are the fundamental building block of the tag-based routing system.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParseTag parses a tag from "key:value" format.
// Returns an error if the format is invalid.
func ParseTag(s string) (Tag, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Tag{}, fmt.Errorf("empty tag string")
	}

	// Special case: bare "*" is treated as a wildcard
	if s == "*" {
		return Tag{Key: "*", Value: "*"}, nil
	}

	// Find separator: first occurrence of ':' or '='
	idx := strings.IndexAny(s, ":=")
	if idx == -1 {
		return Tag{}, fmt.Errorf("invalid tag format: %q (expected key:value or key=value)", s)
	}

	key := strings.TrimSpace(s[:idx])
	value := strings.TrimSpace(s[idx+1:])

	if key == "" {
		return Tag{}, fmt.Errorf("tag key cannot be empty")
	}

	return Tag{Key: key, Value: value}, nil
}

// ParseTags parses a comma-separated list of tags.
// Example: "target:amazon, type:search" -> []Tag{{Key: "target", Value: "amazon"}, {Key: "type", Value: "search"}}
func ParseTags(s string) ([]Tag, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	parts := strings.Split(s, ",")
	tags := make([]Tag, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		tag, err := ParseTag(part)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}

	return tags, nil
}

// String returns the tag in "key:value" format.
func (t Tag) String() string {
	return t.Key + ":" + t.Value
}

// Matches checks if this tag matches the given pattern.
// Supports wildcard matching where pattern value "*" matches any value.
// Both key and value must match (keys are case-sensitive by default).
func (t Tag) Matches(pattern Tag) bool {
	if t.Key != pattern.Key {
		return false
	}
	if pattern.Value == "*" {
		return true
	}
	return t.Value == pattern.Value
}

// MatchesAll returns true if all required tags are satisfied by the given tags.
// Uses AND logic: every required tag must match at least one tag in the list.
func MatchesAll(tags []Tag, required []Tag) bool {
	for _, req := range required {
		found := false
		for _, tag := range tags {
			if tag.Matches(req) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// MatchesNone returns true if none of the excluded tags match any of the given tags.
// Uses NOT logic: none of the excluded tags should be present.
func MatchesNone(tags []Tag, excluded []Tag) bool {
	for _, excl := range excluded {
		for _, tag := range tags {
			if tag.Matches(excl) {
				return false
			}
		}
	}
	return true
}

// TagsToStrings converts a slice of Tags to their string representations.
func TagsToStrings(tags []Tag) []string {
	if tags == nil {
		return nil
	}
	result := make([]string, len(tags))
	for i, tag := range tags {
		result[i] = tag.String()
	}
	return result
}

// StringsToTags parses a slice of strings into Tags.
// Returns an error on the first invalid tag.
func StringsToTags(strs []string) ([]Tag, error) {
	if strs == nil {
		return nil, nil
	}
	tags := make([]Tag, 0, len(strs))
	for _, s := range strs {
		tag, err := ParseTag(s)
		if err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

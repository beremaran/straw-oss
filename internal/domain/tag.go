package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrEmptyTag is returned when a tag string is empty.
	ErrEmptyTag = errors.New("empty tag string")
	// ErrInvalidTagFormat is returned when a tag string has an invalid format.
	ErrInvalidTagFormat = errors.New("invalid tag format")
	// ErrEmptyTagKey is returned when a tag key is empty.
	ErrEmptyTagKey = errors.New("tag key cannot be empty")
)

// Tag represents a key-value pair used for routing and filtering.
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParseTag parses a tag string into a Tag. Supported formats: "key:value", "key=value", or "*" for wildcard.
func ParseTag(s string) (Tag, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Tag{}, ErrEmptyTag
	}

	if s == "*" {
		return Tag{Key: "*", Value: "*"}, nil
	}

	idx := strings.IndexAny(s, ":=")
	if idx == -1 {
		return Tag{}, fmt.Errorf("%w: %q", ErrInvalidTagFormat, s)
	}

	key := strings.TrimSpace(s[:idx])
	value := strings.TrimSpace(s[idx+1:])

	if key == "" {
		return Tag{}, ErrEmptyTagKey
	}

	return Tag{Key: key, Value: value}, nil
}

// ParseTags parses a comma-separated string of tags.
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

func (t Tag) String() string {
	return t.Key + ":" + t.Value
}

// Matches reports whether the tag matches the given pattern tag.
func (t Tag) Matches(pattern Tag) bool {
	if t.Key != pattern.Key {
		return false
	}

	if pattern.Value == "*" {
		return true
	}

	return t.Value == pattern.Value
}

// MatchesAll reports whether all required tags are found in the given tags.
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

// MatchesNone reports that none of the excluded tags match the given tags.
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

// TagsToStrings converts a slice of Tag to string representations.
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

// StringsToTags converts a slice of tag strings to Tag values.
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

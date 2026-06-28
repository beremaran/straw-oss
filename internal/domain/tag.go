package domain

import (
	"fmt"
	"strings"
)

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func ParseTag(s string) (Tag, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Tag{}, fmt.Errorf("empty tag string")
	}

	if s == "*" {
		return Tag{Key: "*", Value: "*"}, nil
	}

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

func (t Tag) Matches(pattern Tag) bool {
	if t.Key != pattern.Key {
		return false
	}
	if pattern.Value == "*" {
		return true
	}

	return t.Value == pattern.Value
}

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

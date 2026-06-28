package router

import (
	"fmt"
	"net/http"

	"github.com/beremaran/straw/internal/domain"
)

const (
	HeaderRelayTags      = "X-Relay-Tags"
	HeaderLegacyRetailer = "X-Straw-Retailer"
	HeaderLegacyMode     = "X-Straw-Mode"
	HeaderLegacyCountry  = "X-Straw-Country"
)

type TagParser struct{}

func NewTagParser() *TagParser {
	return &TagParser{}
}

type ParseResult struct {
	Tags     []domain.Tag
	Warnings []string
}

func (p *TagParser) ParseTags(r *http.Request, apiKey *domain.ApiKey) (*ParseResult, error) {
	result, addTag := newParseResult()

	err := addRelayHeaderTags(r, addTag)
	if err != nil {
		return nil, err
	}
	addLegacyHeaderTags(r, result, addTag)
	err = addScopedTags(apiKey, addTag)
	if err != nil {
		return nil, err
	}

	return result, nil
}

type tagAdder func(domain.Tag)

func newParseResult() (*ParseResult, tagAdder) {
	result := &ParseResult{
		Tags:     make([]domain.Tag, 0),
		Warnings: make([]string, 0),
	}

	tagMap := make(map[domain.Tag]bool)

	addTag := func(t domain.Tag) {
		if !tagMap[t] {
			tagMap[t] = true
			result.Tags = append(result.Tags, t)
		}
	}

	return result, addTag
}

func addRelayHeaderTags(r *http.Request, addTag tagAdder) error {
	if headerVal := r.Header.Get(HeaderRelayTags); headerVal != "" {
		tags, err := domain.ParseTags(headerVal)
		if err != nil {
			return fmt.Errorf("failed to parse %s: %w", HeaderRelayTags, err)
		}
		for _, t := range tags {
			addTag(t)
		}
	}

	return nil
}

type legacyTagHeader struct {
	header string
	key    string
}

var legacyTagHeaders = []legacyTagHeader{
	{header: HeaderLegacyRetailer, key: "target"},
	{header: HeaderLegacyMode, key: "type"},
	{header: HeaderLegacyCountry, key: "region"},
}

func addLegacyHeaderTags(r *http.Request, result *ParseResult, addTag tagAdder) {
	for _, legacy := range legacyTagHeaders {
		val := r.Header.Get(legacy.header)
		if val == "" {
			continue
		}
		addTag(domain.Tag{Key: legacy.key, Value: val})
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %s is deprecated. Use %s: %s=%s instead.", legacy.header, HeaderRelayTags, legacy.key, val))
	}
}

func addScopedTags(apiKey *domain.ApiKey, addTag tagAdder) error {
	if apiKey == nil || len(apiKey.Scopes) == 0 {
		return nil
	}

	scopes, err := domain.StringsToTags(apiKey.Scopes)
	if err != nil {
		return fmt.Errorf("invalid api key scopes: %w", err)
	}
	for _, t := range scopes {
		addTag(t)
	}

	return nil
}

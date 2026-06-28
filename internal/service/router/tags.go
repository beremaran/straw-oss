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

	if headerVal := r.Header.Get(HeaderRelayTags); headerVal != "" {

		tags, err := domain.ParseTags(headerVal)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", HeaderRelayTags, err)
		}
		for _, t := range tags {
			addTag(t)
		}
	}

	if val := r.Header.Get(HeaderLegacyRetailer); val != "" {
		addTag(domain.Tag{Key: "target", Value: val})
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %s is deprecated. Use %s: target=%s instead.", HeaderLegacyRetailer, HeaderRelayTags, val))
	}

	if val := r.Header.Get(HeaderLegacyMode); val != "" {
		addTag(domain.Tag{Key: "type", Value: val})
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %s is deprecated. Use %s: type=%s instead.", HeaderLegacyMode, HeaderRelayTags, val))
	}

	if val := r.Header.Get(HeaderLegacyCountry); val != "" {
		addTag(domain.Tag{Key: "region", Value: val})
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %s is deprecated. Use %s: region=%s instead.", HeaderLegacyCountry, HeaderRelayTags, val))
	}

	if apiKey != nil && len(apiKey.Scopes) > 0 {

		scopes, err := domain.StringsToTags(apiKey.Scopes)
		if err != nil {

			return nil, fmt.Errorf("invalid api key scopes: %w", err)
		}
		for _, t := range scopes {
			addTag(t)
		}
	}

	return result, nil
}

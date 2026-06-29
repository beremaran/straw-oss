package dto

import (
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/domain"
	"github.com/beremaran/straw/pkg/protocol"
)

// ToProtocolRequest converts a RelayRequest to a protocol.Request.
func (r *RelayRequest) ToProtocolRequest() (*protocol.Request, error) {
	var timeout time.Duration

	if r.Timeout != "" {
		var err error

		timeout, err = time.ParseDuration(r.Timeout)
		if err != nil {
			return nil, fmt.Errorf("parse timeout: %w", err)
		}
	}

	headers := make(protocol.HeaderMap, 0, len(r.Headers))
	for k, v := range r.Headers {
		headers = append(headers, protocol.Header{Key: k, Value: v})
	}

	return &protocol.Request{
		ID:              r.ID,
		Method:          r.Method,
		URL:             r.URL,
		Headers:         headers,
		Body:            r.Body,
		Timeout:         timeout,
		SessionID:       r.SessionID,
		TraceID:         r.TraceID,
		StreamResponse:  r.StreamResponse,
		MaxResponseSize: r.MaxResponseSize,
	}, nil
}

// FromProtocolResponse converts a protocol.Response to a RelayResponse.
func FromProtocolResponse(resp *protocol.Response, meta *RelayMetaDTO) *RelayResponse {
	headers := make(map[string]string, len(resp.Headers))
	for _, h := range resp.Headers {
		headers[h.Key] = h.Value
	}

	var timing *TimingDTO
	if resp.Timing != nil {
		timing = &TimingDTO{
			DNSLookup:    resp.Timing.DNSLookup.String(),
			TCPConnect:   resp.Timing.TCPConnect.String(),
			TLSHandshake: resp.Timing.TLSHandshake.String(),
			FirstByte:    resp.Timing.FirstByte.String(),
			Total:        resp.Timing.Total.String(),
		}
	}

	return &RelayResponse{
		RequestID:   resp.RequestID,
		StatusCode:  resp.StatusCode,
		Headers:     headers,
		Body:        resp.Body,
		SessionID:   resp.SessionID,
		Timing:      timing,
		Meta:        meta,
		IsStreaming: resp.IsStreaming,
	}
}

// ToDomain converts a CreateRoutingRuleRequest to a domain.RoutingRule.
func (r *CreateRoutingRuleRequest) ToDomain() (*domain.RoutingRule, error) {
	var hardTimeout time.Duration

	if r.HardTimeout != "" {
		var err error

		hardTimeout, err = time.ParseDuration(r.HardTimeout)
		if err != nil {
			return nil, fmt.Errorf("parse hard timeout: %w", err)
		}
	}

	rule := &domain.RoutingRule{
		Name:                 r.Name,
		RequiredTags:         r.RequiredTags,
		ExcludedTags:         r.ExcludedTags,
		Priority:             r.Priority,
		HardTimeout:          hardTimeout,
		RateLimitPerMinute:   r.RateLimitPerMinute,
		RateLimitPerSecond:   r.RateLimitPerSecond,
		AllowedEndpointTypes: r.AllowedEndpointTypes,
		RequiredEndpointCaps: r.RequiredEndpointCaps,
		FingerprintPreset:    r.FingerprintPreset,
		QuotaKey:             r.QuotaKey,
		AllowInsecureTLS:     r.AllowInsecureTLS,
		PinnedCertHash:       r.PinnedCertHash,
		IsActive:             r.IsActive,
	}

	if r.FingerprintABTest != nil {
		rule.FingerprintABTest = r.FingerprintABTest.ToDomain()
	}

	if r.RequestFilters != nil {
		rule.RequestFilters = r.RequestFilters.ToDomain()
	}

	if len(r.EndpointPools) > 0 {
		rule.EndpointPools = EndpointPoolsDTOToDomain(r.EndpointPools)
	}

	return rule, nil
}

// ToDomain converts an ABConfigDTO to a domain.ABConfig.
func (c *ABConfigDTO) ToDomain() *domain.ABConfig {
	if c == nil {
		return nil
	}

	variants := make([]domain.ABVariant, len(c.Variants))
	for i, v := range c.Variants {
		variants[i] = domain.ABVariant{
			Fingerprint: v.Fingerprint,
			Weight:      v.Weight,
		}
	}

	return &domain.ABConfig{
		Variants: variants,
		Strategy: c.Strategy,
	}
}

// ToDomain converts a RequestFilterDTO to a domain.RequestFilter.
func (f *RequestFilterDTO) ToDomain() *domain.RequestFilter {
	if f == nil {
		return nil
	}

	return &domain.RequestFilter{
		BlockContentTypes: f.BlockContentTypes,
		BlockURLPatterns:  f.BlockURLPatterns,
		BlockDomains:      f.BlockDomains,
		EnableAdblock:     f.EnableAdblock,
		AdblockLists:      f.AdblockLists,
	}
}

// EndpointPoolsDTOToDomain converts a slice of EndpointPoolDTO to domain.EndpointPool.
func EndpointPoolsDTOToDomain(pools []EndpointPoolDTO) []domain.EndpointPool {
	result := make([]domain.EndpointPool, len(pools))
	for i, p := range pools {
		result[i] = domain.EndpointPool{
			Tier:       p.Tier,
			Endpoints:  p.Endpoints,
			MaxRetries: p.MaxRetries,
		}
	}

	return result
}

// FromRoutingRule converts a domain.RoutingRule to a RoutingRuleResponse.
func FromRoutingRule(rule *domain.RoutingRule) *RoutingRuleResponse {
	if rule == nil {
		return nil
	}

	resp := &RoutingRuleResponse{
		ID:                   rule.ID,
		Name:                 rule.Name,
		RequiredTags:         rule.RequiredTags,
		ExcludedTags:         rule.ExcludedTags,
		Priority:             rule.Priority,
		RateLimitPerMinute:   rule.RateLimitPerMinute,
		RateLimitPerSecond:   rule.RateLimitPerSecond,
		AllowedEndpointTypes: rule.AllowedEndpointTypes,
		RequiredEndpointCaps: rule.RequiredEndpointCaps,
		FingerprintPreset:    rule.FingerprintPreset,
		QuotaKey:             rule.QuotaKey,
		AllowInsecureTLS:     rule.AllowInsecureTLS,
		PinnedCertHash:       rule.PinnedCertHash,
		IsActive:             rule.IsActive,
		Version:              rule.Version,
		CreatedAt:            rule.CreatedAt,
		UpdatedAt:            rule.UpdatedAt,
	}

	if rule.HardTimeout > 0 {
		resp.HardTimeout = rule.HardTimeout.String()
	}

	if rule.FingerprintABTest != nil {
		resp.FingerprintABTest = FromABConfig(rule.FingerprintABTest)
	}

	if rule.RequestFilters != nil {
		resp.RequestFilters = FromRequestFilter(rule.RequestFilters)
	}

	if len(rule.EndpointPools) > 0 {
		resp.EndpointPools = FromEndpointPools(rule.EndpointPools)
	}

	return resp
}

// FromRoutingRules converts a slice of domain.RoutingRule to RoutingRuleResponse.
func FromRoutingRules(rules []domain.RoutingRule) []RoutingRuleResponse {
	result := make([]RoutingRuleResponse, len(rules))
	for i, r := range rules {
		resp := FromRoutingRule(&r)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

// FromABConfig converts a domain.ABConfig to an ABConfigDTO.
func FromABConfig(c *domain.ABConfig) *ABConfigDTO {
	if c == nil {
		return nil
	}

	variants := make([]ABVariantDTO, len(c.Variants))
	for i, v := range c.Variants {
		variants[i] = ABVariantDTO{
			Fingerprint: v.Fingerprint,
			Weight:      v.Weight,
		}
	}

	return &ABConfigDTO{
		Variants: variants,
		Strategy: c.Strategy,
	}
}

// FromRequestFilter converts a domain.RequestFilter to a RequestFilterDTO.
func FromRequestFilter(f *domain.RequestFilter) *RequestFilterDTO {
	if f == nil {
		return nil
	}

	return &RequestFilterDTO{
		BlockContentTypes: f.BlockContentTypes,
		BlockURLPatterns:  f.BlockURLPatterns,
		BlockDomains:      f.BlockDomains,
		EnableAdblock:     f.EnableAdblock,
		AdblockLists:      f.AdblockLists,
	}
}

// FromEndpointPools converts a slice of domain.EndpointPool to EndpointPoolDTO.
func FromEndpointPools(pools []domain.EndpointPool) []EndpointPoolDTO {
	result := make([]EndpointPoolDTO, len(pools))
	for i, p := range pools {
		result[i] = EndpointPoolDTO{
			Tier:       p.Tier,
			Endpoints:  p.Endpoints,
			MaxRetries: p.MaxRetries,
		}
	}

	return result
}

// FromAPIKey converts a domain.APIKey to an APIKeyResponse.
func FromAPIKey(key *domain.APIKey) *APIKeyResponse {
	if key == nil {
		return nil
	}

	return &APIKeyResponse{
		ID:                key.ID,
		Name:              key.Name,
		Scopes:            key.Scopes,
		RateLimitOverride: key.RateLimitOverride,
		IsActive:          key.IsActive,
		CreatedAt:         key.CreatedAt,
		ExpiresAt:         key.ExpiresAt,
	}
}

// FromAPIKeys converts a slice of domain.APIKey to APIKeyResponse.
func FromAPIKeys(keys []domain.APIKey) []APIKeyResponse {
	result := make([]APIKeyResponse, len(keys))
	for i, k := range keys {
		resp := FromAPIKey(&k)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

// FromAPIKeyToken converts a domain.APIKeyToken to an APIKeyTokenResponse.
func FromAPIKeyToken(token *domain.APIKeyToken) *APIKeyTokenResponse {
	if token == nil {
		return nil
	}

	return &APIKeyTokenResponse{
		ID:        token.ID,
		Status:    string(token.Status),
		ExpiresAt: token.ExpiresAt,
		CreatedAt: token.CreatedAt,
	}
}

// FromAPIKeyTokens converts a slice of domain.APIKeyToken to APIKeyTokenResponse.
func FromAPIKeyTokens(tokens []domain.APIKeyToken) []APIKeyTokenResponse {
	result := make([]APIKeyTokenResponse, len(tokens))
	for i, token := range tokens {
		resp := FromAPIKeyToken(&token)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

// ToDomain converts a CreateFingerprintRequest to a domain.FingerprintPreset.
func (r *CreateFingerprintRequest) ToDomain() *domain.FingerprintPreset {
	return &domain.FingerprintPreset{
		ID:     r.ID,
		Name:   r.Name,
		Config: domain.ConfigMap(r.Config),
	}
}

// FromFingerprintPreset converts a domain.FingerprintPreset to a FingerprintResponse.
func FromFingerprintPreset(p *domain.FingerprintPreset) *FingerprintResponse {
	if p == nil {
		return nil
	}

	return &FingerprintResponse{
		ID:        p.ID,
		Name:      p.Name,
		Config:    map[string]any(p.Config),
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// FromFingerprintPresets converts a slice of domain.FingerprintPreset to FingerprintResponse.
func FromFingerprintPresets(presets []domain.FingerprintPreset) []FingerprintResponse {
	result := make([]FingerprintResponse, len(presets))
	for i, p := range presets {
		resp := FromFingerprintPreset(&p)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

// FromUsageSummary converts a domain.UsageSummary to a UsageSummaryDTO.
func FromUsageSummary(s *domain.UsageSummary) *UsageSummaryDTO {
	if s == nil {
		return nil
	}

	return &UsageSummaryDTO{
		Date:          s.Date,
		TotalRequests: s.TotalRequests,
		TotalBytes:    s.TotalBytes,
		CostUnits:     s.CostUnits,
		Breakdown:     s.Breakdown,
	}
}

// FromUsageSummaries converts a slice of domain.UsageSummary to UsageSummaryDTO.
func FromUsageSummaries(summaries []domain.UsageSummary) []UsageSummaryDTO {
	result := make([]UsageSummaryDTO, len(summaries))
	for i, s := range summaries {
		resp := FromUsageSummary(&s)
		if resp != nil {
			result[i] = *resp
		}
	}

	return result
}

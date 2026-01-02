package filter

import (
	"context"
	"testing"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestNewFilterRequest(t *testing.T) {
	req := NewFilterRequest("https://example.com/page", "example.com", "text/html", "GET")

	assert.Equal(t, "https://example.com/page", req.URL)
	assert.Equal(t, "example.com", req.Host)
	assert.Equal(t, "text/html", req.ContentType)
	assert.Equal(t, "GET", req.Method)
}

func TestService_ShouldBlock_NilFilter(t *testing.T) {
	service := NewService(nil)
	req := NewFilterRequest("https://example.com/page", "example.com", "text/html", "GET")

	result, err := service.ShouldBlock(context.Background(), req, nil)

	assert.NoError(t, err)
	assert.False(t, result.Blocked)
}

func TestService_ShouldBlock_EmptyFilter(t *testing.T) {
	service := NewService(nil)
	req := NewFilterRequest("https://example.com/page", "example.com", "text/html", "GET")
	filter := &domain.RequestFilter{}

	result, err := service.ShouldBlock(context.Background(), req, filter)

	assert.NoError(t, err)
	assert.False(t, result.Blocked)
}

func TestService_ContentTypeBlocking(t *testing.T) {
	service := NewService(nil)

	tests := []struct {
		name        string
		contentType string
		patterns    []string
		wantBlocked bool
	}{
		{
			name:        "image/* blocks image/png",
			contentType: "image/png",
			patterns:    []string{"image/*"},
			wantBlocked: true,
		},
		{
			name:        "image/* blocks image/jpeg",
			contentType: "image/jpeg",
			patterns:    []string{"image/*"},
			wantBlocked: true,
		},
		{
			name:        "font/* blocks font/woff2",
			contentType: "font/woff2",
			patterns:    []string{"font/*"},
			wantBlocked: true,
		},
		{
			name:        "video/* blocks video/mp4",
			contentType: "video/mp4",
			patterns:    []string{"video/*"},
			wantBlocked: true,
		},
		{
			name:        "text/html not blocked by image/*",
			contentType: "text/html",
			patterns:    []string{"image/*"},
			wantBlocked: false,
		},
		{
			name:        "exact match works",
			contentType: "application/javascript",
			patterns:    []string{"application/javascript"},
			wantBlocked: true,
		},
		{
			name:        "content-type with charset",
			contentType: "text/html; charset=utf-8",
			patterns:    []string{"text/html"},
			wantBlocked: true,
		},
		{
			name:        "multiple patterns - first match",
			contentType: "image/png",
			patterns:    []string{"video/*", "image/*", "font/*"},
			wantBlocked: true,
		},
		{
			name:        "case insensitive",
			contentType: "IMAGE/PNG",
			patterns:    []string{"image/*"},
			wantBlocked: true,
		},
		{
			name:        "empty content-type not blocked",
			contentType: "",
			patterns:    []string{"image/*"},
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := NewFilterRequest("https://example.com/file", "example.com", tt.contentType, "GET")
			filter := &domain.RequestFilter{
				BlockContentTypes: tt.patterns,
			}

			result, err := service.ShouldBlock(context.Background(), req, filter)

			assert.NoError(t, err)
			assert.Equal(t, tt.wantBlocked, result.Blocked)
			if tt.wantBlocked {
				assert.Equal(t, FilterTypeContentType, result.FilterType)
				assert.Contains(t, result.Reason, "content-type:")
			}
		})
	}
}

func TestService_URLPatternBlocking(t *testing.T) {
	service := NewService(nil)

	tests := []struct {
		name        string
		url         string
		patterns    []string
		wantBlocked bool
	}{
		{
			name:        "*/ads/* blocks ads path",
			url:         "https://example.com/ads/banner.js",
			patterns:    []string{"*/ads/*"},
			wantBlocked: true,
		},
		{
			name:        "*.google-analytics.com blocks subdomain",
			url:         "https://www.google-analytics.com/analytics.js",
			patterns:    []string{"*.google-analytics.com"},
			wantBlocked: true,
		},
		{
			name:        "wildcard domain in path",
			url:         "https://example.com/static/ads/image.png",
			patterns:    []string{"*/static/ads/*"},
			wantBlocked: true,
		},
		{
			name:        "no match",
			url:         "https://example.com/products/item",
			patterns:    []string{"*/ads/*"},
			wantBlocked: false,
		},
		{
			name:        "tracking pixel",
			url:         "https://tracking.example.com/pixel.gif",
			patterns:    []string{"*pixel*"},
			wantBlocked: true,
		},
		{
			name:        "empty URL not blocked",
			url:         "",
			patterns:    []string{"*/ads/*"},
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := ""
			if tt.url != "" {
				host = "example.com"
			}
			req := NewFilterRequest(tt.url, host, "text/html", "GET")
			filter := &domain.RequestFilter{
				BlockURLPatterns: tt.patterns,
			}

			result, err := service.ShouldBlock(context.Background(), req, filter)

			assert.NoError(t, err)
			assert.Equal(t, tt.wantBlocked, result.Blocked)
			if tt.wantBlocked {
				assert.Equal(t, FilterTypeURLPattern, result.FilterType)
				assert.Contains(t, result.Reason, "url-pattern:")
			}
		})
	}
}

func TestService_DomainBlocking(t *testing.T) {
	service := NewService(nil)

	tests := []struct {
		name        string
		host        string
		domains     []string
		wantBlocked bool
	}{
		{
			name:        "exact domain match",
			host:        "googleanalytics.com",
			domains:     []string{"googleanalytics.com"},
			wantBlocked: true,
		},
		{
			name:        "subdomain blocked by parent",
			host:        "www.googleanalytics.com",
			domains:     []string{"googleanalytics.com"},
			wantBlocked: true,
		},
		{
			name:        "wildcard subdomain",
			host:        "stats.ads.example.com",
			domains:     []string{"*.ads.example.com"},
			wantBlocked: true,
		},
		{
			name:        "different domain not blocked",
			host:        "example.com",
			domains:     []string{"googleanalytics.com"},
			wantBlocked: false,
		},
		{
			name:        "host with port",
			host:        "googleanalytics.com:443",
			domains:     []string{"googleanalytics.com"},
			wantBlocked: true,
		},
		{
			name:        "case insensitive",
			host:        "GoogleAnalytics.COM",
			domains:     []string{"googleanalytics.com"},
			wantBlocked: true,
		},
		{
			name:        "empty host not blocked",
			host:        "",
			domains:     []string{"googleanalytics.com"},
			wantBlocked: false,
		},
		{
			name:        "multiple domains - match second",
			host:        "facebook.com",
			domains:     []string{"googleanalytics.com", "facebook.com", "twitter.com"},
			wantBlocked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := NewFilterRequest("https://"+tt.host+"/path", tt.host, "text/html", "GET")
			filter := &domain.RequestFilter{
				BlockDomains: tt.domains,
			}

			result, err := service.ShouldBlock(context.Background(), req, filter)

			assert.NoError(t, err)
			assert.Equal(t, tt.wantBlocked, result.Blocked)
			if tt.wantBlocked {
				assert.Equal(t, FilterTypeDomain, result.FilterType)
				assert.Contains(t, result.Reason, "domain:")
			}
		})
	}
}

func TestService_CombinedFilters(t *testing.T) {
	service := NewService(nil)

	t.Run("content-type checked first", func(t *testing.T) {
		req := NewFilterRequest("https://ads.example.com/banner.png", "ads.example.com", "image/png", "GET")
		filter := &domain.RequestFilter{
			BlockContentTypes: []string{"image/*"},
			BlockDomains:      []string{"ads.example.com"},
		}

		result, err := service.ShouldBlock(context.Background(), req, filter)

		assert.NoError(t, err)
		assert.True(t, result.Blocked)
		// Content-type is checked first
		assert.Equal(t, FilterTypeContentType, result.FilterType)
	})

	t.Run("URL pattern checked after content-type", func(t *testing.T) {
		req := NewFilterRequest("https://example.com/ads/script.js", "example.com", "application/javascript", "GET")
		filter := &domain.RequestFilter{
			BlockContentTypes: []string{"image/*"},
			BlockURLPatterns:  []string{"*/ads/*"},
		}

		result, err := service.ShouldBlock(context.Background(), req, filter)

		assert.NoError(t, err)
		assert.True(t, result.Blocked)
		assert.Equal(t, FilterTypeURLPattern, result.FilterType)
	})

	t.Run("domain checked after URL pattern", func(t *testing.T) {
		req := NewFilterRequest("https://tracking.com/pixel.gif", "tracking.com", "image/gif", "GET")
		filter := &domain.RequestFilter{
			BlockContentTypes: []string{"video/*"},
			BlockURLPatterns:  []string{"*/ads/*"},
			BlockDomains:      []string{"tracking.com"},
		}

		result, err := service.ShouldBlock(context.Background(), req, filter)

		assert.NoError(t, err)
		assert.True(t, result.Blocked)
		assert.Equal(t, FilterTypeDomain, result.FilterType)
	})
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		s       string
		pattern string
		want    bool
	}{
		{"image/png", "image/*", true},
		{"image/jpeg", "image/*", true},
		{"text/html", "image/*", false},
		{"application/javascript", "application/javascript", true},
		{"video/mp4", "video/*", true},
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.pattern, func(t *testing.T) {
			got := matchGlob(tt.s, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchDomain(t *testing.T) {
	tests := []struct {
		host    string
		pattern string
		want    bool
	}{
		{"example.com", "example.com", true},
		{"www.example.com", "example.com", true},
		{"sub.www.example.com", "example.com", true},
		{"notexample.com", "example.com", false},
		{"sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false}, // wildcard requires subdomain
	}

	for _, tt := range tests {
		t.Run(tt.host+"_"+tt.pattern, func(t *testing.T) {
			got := matchDomain(tt.host, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMatchURLPattern(t *testing.T) {
	tests := []struct {
		url     string
		pattern string
		want    bool
	}{
		{"https://example.com/ads/banner.js", "*/ads/*", true},
		{"https://example.com/static/ads/foo.js", "*/ads/*", true},
		{"https://example.com/products/item", "*/ads/*", false},
		{"https://www.google-analytics.com/foo", "*.google-analytics.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.url+"_"+tt.pattern, func(t *testing.T) {
			got := matchURLPattern(tt.url, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

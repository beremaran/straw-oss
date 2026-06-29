package filter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testImagePng              = "image/png"
	testImageWildcard         = "image/*"
	testVideoWildcard         = "video/*"
	testTextHTML              = "text/html"
	testApplicationJS         = "application/javascript"
	testAdsWildcard           = "*/ads/*"
	testGoogleAnalyticsDomain = "googleanalytics.com"
)

func TestNewFilterRequest(t *testing.T) {
	req := NewFilterRequest("https://example.com/page", testExampleHost, testTextHTML, "GET")

	assert.Equal(t, "https://example.com/page", req.URL)
	assert.Equal(t, testExampleHost, req.Host)
	assert.Equal(t, testTextHTML, req.ContentType)
	assert.Equal(t, "GET", req.Method)
}

func TestService_ShouldBlock_NilFilter(t *testing.T) {
	service := NewService(nil)
	req := NewFilterRequest("https://example.com/page", testExampleHost, testTextHTML, "GET")

	result, err := service.ShouldBlock(context.Background(), req, nil)

	require.NoError(t, err)
	assert.False(t, result.Blocked)
}

func TestService_ShouldBlock_EmptyFilter(t *testing.T) {
	service := NewService(nil)
	req := NewFilterRequest("https://example.com/page", testExampleHost, testTextHTML, "GET")
	filter := &domain.RequestFilter{}

	result, err := service.ShouldBlock(context.Background(), req, filter)

	require.NoError(t, err)
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
			contentType: testImagePng,
			patterns:    []string{testImageWildcard},
			wantBlocked: true,
		},
		{
			name:        "image/* blocks image/jpeg",
			contentType: "image/jpeg",
			patterns:    []string{testImageWildcard},
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
			patterns:    []string{testVideoWildcard},
			wantBlocked: true,
		},
		{
			name:        "text/html not blocked by image/*",
			contentType: testTextHTML,
			patterns:    []string{testImageWildcard},
			wantBlocked: false,
		},
		{
			name:        "exact match works",
			contentType: testApplicationJS,
			patterns:    []string{testApplicationJS},
			wantBlocked: true,
		},
		{
			name:        "content-type with charset",
			contentType: "text/html; charset=utf-8",
			patterns:    []string{testTextHTML},
			wantBlocked: true,
		},
		{
			name:        "multiple patterns - first match",
			contentType: testImagePng,
			patterns:    []string{testVideoWildcard, testImageWildcard, "font/*"},
			wantBlocked: true,
		},
		{
			name:        "case insensitive",
			contentType: "IMAGE/PNG",
			patterns:    []string{testImageWildcard},
			wantBlocked: true,
		},
		{
			name:        "empty content-type not blocked",
			contentType: "",
			patterns:    []string{testImageWildcard},
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

			require.NoError(t, err)
			assert.Equal(t, tt.wantBlocked, result.Blocked)
			if tt.wantBlocked {
				assert.Equal(t, TypeContentType, result.FilterType)
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
			patterns:    []string{testAdsWildcard},
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
			url:         testNormalURL,
			patterns:    []string{testAdsWildcard},
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
			patterns:    []string{testAdsWildcard},
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := ""
			if tt.url != "" {
				host = testExampleHost
			}
			req := NewFilterRequest(tt.url, host, testTextHTML, "GET")
			filter := &domain.RequestFilter{
				BlockURLPatterns: tt.patterns,
			}

			result, err := service.ShouldBlock(context.Background(), req, filter)

			require.NoError(t, err)
			assert.Equal(t, tt.wantBlocked, result.Blocked)
			if tt.wantBlocked {
				assert.Equal(t, TypeURLPattern, result.FilterType)
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
			host:        testGoogleAnalyticsDomain,
			domains:     []string{testGoogleAnalyticsDomain},
			wantBlocked: true,
		},
		{
			name:        "subdomain blocked by parent",
			host:        "www.googleanalytics.com",
			domains:     []string{testGoogleAnalyticsDomain},
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
			host:        testExampleHost,
			domains:     []string{testGoogleAnalyticsDomain},
			wantBlocked: false,
		},
		{
			name:        "host with port",
			host:        "googleanalytics.com:443",
			domains:     []string{testGoogleAnalyticsDomain},
			wantBlocked: true,
		},
		{
			name:        "case insensitive",
			host:        "GoogleAnalytics.COM",
			domains:     []string{testGoogleAnalyticsDomain},
			wantBlocked: true,
		},
		{
			name:        "empty host not blocked",
			host:        "",
			domains:     []string{testGoogleAnalyticsDomain},
			wantBlocked: false,
		},
		{
			name:        "multiple domains - match second",
			host:        "facebook.com",
			domains:     []string{testGoogleAnalyticsDomain, "facebook.com", "twitter.com"},
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

			require.NoError(t, err)
			assert.Equal(t, tt.wantBlocked, result.Blocked)
			if tt.wantBlocked {
				assert.Equal(t, TypeDomain, result.FilterType)
				assert.Contains(t, result.Reason, "domain:")
			}
		})
	}
}

func TestService_CombinedFilters(t *testing.T) {
	service := NewService(nil)

	t.Run("content-type checked first", func(t *testing.T) {
		req := NewFilterRequest("https://ads.example.com/banner.png", "ads.example.com", testImagePng, "GET")
		filter := &domain.RequestFilter{
			BlockContentTypes: []string{testImageWildcard},
			BlockDomains:      []string{"ads.example.com"},
		}

		result, err := service.ShouldBlock(context.Background(), req, filter)

		require.NoError(t, err)
		assert.True(t, result.Blocked)

		assert.Equal(t, TypeContentType, result.FilterType)
	})

	t.Run("URL pattern checked after content-type", func(t *testing.T) {
		req := NewFilterRequest("https://example.com/ads/script.js", "example.com", testApplicationJS, "GET")
		filter := &domain.RequestFilter{
			BlockContentTypes: []string{testImageWildcard},
			BlockURLPatterns:  []string{testAdsWildcard},
		}

		result, err := service.ShouldBlock(context.Background(), req, filter)

		require.NoError(t, err)
		assert.True(t, result.Blocked)
		assert.Equal(t, TypeURLPattern, result.FilterType)
	})

	t.Run("domain checked after URL pattern", func(t *testing.T) {
		req := NewFilterRequest("https://tracking.com/pixel.gif", "tracking.com", "image/gif", "GET")
		filter := &domain.RequestFilter{
			BlockContentTypes: []string{testVideoWildcard},
			BlockURLPatterns:  []string{testAdsWildcard},
			BlockDomains:      []string{"tracking.com"},
		}

		result, err := service.ShouldBlock(context.Background(), req, filter)

		require.NoError(t, err)
		assert.True(t, result.Blocked)
		assert.Equal(t, TypeDomain, result.FilterType)
	})
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		s       string
		pattern string
		want    bool
	}{
		{testImagePng, testImageWildcard, true},
		{"image/jpeg", testImageWildcard, true},
		{testTextHTML, testImageWildcard, false},
		{testApplicationJS, testApplicationJS, true},
		{"video/mp4", testVideoWildcard, true},
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
		{testExampleHost, testExampleHost, true},
		{"www.example.com", testExampleHost, true},
		{"sub.www.example.com", testExampleHost, true},
		{"notexample.com", "example.com", false},
		{"sub.example.com", "*.example.com", true},
		{"example.com", "*.example.com", false},
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
		{"https://example.com/ads/banner.js", testAdsWildcard, true},
		{"https://example.com/static/ads/foo.js", testAdsWildcard, true},
		{testNormalURL, testAdsWildcard, false},
		{"https://www.google-analytics.com/foo", "*.google-analytics.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.url+"_"+tt.pattern, func(t *testing.T) {
			got := matchURLPattern(tt.url, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

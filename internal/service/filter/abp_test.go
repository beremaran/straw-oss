package filter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sample ABP rules for testing
const sampleABPList = `[Adblock Plus 2.0]
! Title: Test Filter List
! Last modified: 2026-01-01
||googleanalytics.com^
||doubleclick.net^
||facebook.com/tr/*
||ads.example.com^
`

func TestNewABPMatcher(t *testing.T) {
	matcher := NewABPMatcher(nil, DefaultABPMatcherConfig())

	assert.NotNil(t, matcher)
	assert.NotNil(t, matcher.matchers)
	assert.NotNil(t, matcher.httpClient)
	assert.Equal(t, 24*time.Hour, matcher.updateInterval)
}

func TestABPMatcher_ParseAndStore(t *testing.T) {
	matcher := NewABPMatcher(nil, DefaultABPMatcherConfig())

	err := matcher.parseAndStore("testlist", stringReader(sampleABPList))

	require.NoError(t, err)
	assert.True(t, matcher.HasList("testlist"))
}

func TestABPMatcher_Match(t *testing.T) {
	matcher := NewABPMatcher(nil, DefaultABPMatcherConfig())
	err := matcher.parseAndStore("testlist", stringReader(sampleABPList))
	require.NoError(t, err)

	tests := []struct {
		name        string
		url         string
		wantBlocked bool
	}{
		{
			name:        "blocks googleanalytics.com",
			url:         "https://www.googleanalytics.com/analytics.js",
			wantBlocked: true,
		},
		{
			name:        "blocks doubleclick.net",
			url:         "https://ad.doubleclick.net/banner",
			wantBlocked: true,
		},
		{
			name:        "blocks facebook tracking",
			url:         "https://www.facebook.com/tr/pixel.gif",
			wantBlocked: true,
		},
		{
			name:        "blocks ads domain",
			url:         "https://ads.example.com/banner.js",
			wantBlocked: true,
		},
		{
			name:        "allows normal content",
			url:         "https://example.com/products/item",
			wantBlocked: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, _ := matcher.Match(tt.url, []string{"testlist"})
			assert.Equal(t, tt.wantBlocked, blocked)
		})
	}
}

func TestABPMatcher_Match_NonexistentList(t *testing.T) {
	matcher := NewABPMatcher(nil, DefaultABPMatcherConfig())

	blocked, rule := matcher.Match("https://googleanalytics.com/foo", []string{"nonexistent"})

	assert.False(t, blocked)
	assert.Empty(t, rule)
}

func TestABPMatcher_LoadList_FromServer(t *testing.T) {
	// Create a test server serving ABP list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleABPList))
	}))
	defer server.Close()

	matcher := NewABPMatcher(nil, ABPMatcherConfig{
		UpdateInterval: time.Hour,
		HTTPTimeout:    5 * time.Second,
	})

	err := matcher.LoadList(context.Background(), "testlist", server.URL)

	require.NoError(t, err)
	assert.True(t, matcher.HasList("testlist"))

	// Verify matching works
	blocked, _ := matcher.Match("https://googleanalytics.com/foo", []string{"testlist"})
	assert.True(t, blocked)
}

func TestABPMatcher_LoadList_HTTPError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	matcher := NewABPMatcher(nil, ABPMatcherConfig{
		UpdateInterval: time.Hour,
		HTTPTimeout:    5 * time.Second,
	})

	err := matcher.LoadList(context.Background(), "testlist", server.URL)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status code")
}

func TestABPMatcher_HasList(t *testing.T) {
	matcher := NewABPMatcher(nil, DefaultABPMatcherConfig())

	assert.False(t, matcher.HasList("nonexistent"))

	_ = matcher.parseAndStore("testlist", stringReader(sampleABPList))
	assert.True(t, matcher.HasList("testlist"))
}

func TestABPMatcher_MultipleLists(t *testing.T) {
	matcher := NewABPMatcher(nil, DefaultABPMatcherConfig())

	list1 := `[Adblock Plus 2.0]
||ads.example.com^
`
	list2 := `[Adblock Plus 2.0]
||tracking.example.com^
`

	err := matcher.parseAndStore("list1", stringReader(list1))
	require.NoError(t, err)

	err = matcher.parseAndStore("list2", stringReader(list2))
	require.NoError(t, err)

	// Test matching against specific lists
	blocked1, _ := matcher.Match("https://ads.example.com/banner", []string{"list1"})
	assert.True(t, blocked1)

	blocked2, _ := matcher.Match("https://tracking.example.com/pixel", []string{"list2"})
	assert.True(t, blocked2)

	// list1 shouldn't block tracking.example.com
	blocked3, _ := matcher.Match("https://tracking.example.com/pixel", []string{"list1"})
	assert.False(t, blocked3)

	// Both lists together
	blocked4, _ := matcher.Match("https://ads.example.com/banner", []string{"list1", "list2"})
	assert.True(t, blocked4)
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/path", "example.com"},
		{"http://www.example.com:8080/path", "www.example.com"},
		{"https://sub.domain.example.com/", "sub.domain.example.com"},
		{"example.com/path", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := extractDomain(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultLists(t *testing.T) {
	// Verify default lists are defined
	assert.Contains(t, DefaultLists, "easylist")
	assert.Contains(t, DefaultLists, "easyprivacy")
	assert.Contains(t, DefaultLists, "ublock")

	assert.Equal(t, EasyListURL, DefaultLists["easylist"])
	assert.Equal(t, EasyPrivacyURL, DefaultLists["easyprivacy"])
}

// Helper to create a reader from a string
func stringReader(s string) io.Reader {
	return strings.NewReader(s)
}

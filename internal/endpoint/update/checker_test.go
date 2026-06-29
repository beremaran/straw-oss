package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testManifestVersion = "1.0.0"
	testManifestURL     = "http://example.com/binary"
	testManifestSHA256  = "abc123"
	testNewVersion      = "2.0.0"
	testNewVersion2     = "v2.3.4"
	testV100            = "v1.0.0"
)

func TestChecker_New(t *testing.T) {
	c := NewChecker("http://example.com/manifest.json", "1.0.0")

	if c.updateURL != "http://example.com/manifest.json" {
		t.Errorf("expected update URL to be set, got %q", c.updateURL)
	}

	if c.currentVersion != testV100 {
		t.Errorf("expected version 'v1.0.0', got %q", c.currentVersion)
	}

	if c.interval != DefaultCheckInterval {
		t.Errorf("expected default interval %v, got %v", DefaultCheckInterval, c.interval)
	}
}

func TestChecker_NewWithOptions(t *testing.T) {
	customInterval := 1 * time.Minute
	var callbackCalled bool

	c := NewChecker("http://example.com/manifest.json", "v2.0.0",
		WithCheckInterval(customInterval),
		WithHTTPTimeout(10*time.Second),
		WithUpdateCallback(func(*Result) bool {
			callbackCalled = true

			return true
		}),
	)

	if c.interval != customInterval {
		t.Errorf("expected interval %v, got %v", customInterval, c.interval)
	}

	if c.httpTimeout != 10*time.Second {
		t.Errorf("expected timeout 10s, got %v", c.httpTimeout)
	}

	c.callback(&Result{})
	if !callbackCalled {
		t.Error("expected callback to be called")
	}
}

func TestChecker_CheckNow_NoUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		manifest := VersionManifest{
			Version: testManifestVersion,
			URL:     testManifestURL,
			SHA256:  testManifestSHA256,
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0")

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UpdateAvailable {
		t.Error("expected no update available")
	}

	if result.CurrentVersion != testV100 {
		t.Errorf("expected current version 'v1.0.0', got %q", result.CurrentVersion)
	}

	if result.NewVersion != "1.0.0" {
		t.Errorf("expected new version '1.0.0', got %q", result.NewVersion)
	}
}

func TestChecker_CheckNow_UpdateAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		manifest := VersionManifest{
			Version: testNewVersion,
			URL:     "http://example.com/binary-v2",
			SHA256:  "def456",
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0")

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.UpdateAvailable {
		t.Error("expected update available")
	}

	if result.NewVersion != "2.0.0" {
		t.Errorf("expected new version '2.0.0', got %q", result.NewVersion)
	}

	if result.DownloadURL != "http://example.com/binary-v2" {
		t.Errorf("expected download URL 'http://example.com/binary-v2', got %q", result.DownloadURL)
	}

	if result.Checksum != "def456" {
		t.Errorf("expected checksum 'def456', got %q", result.Checksum)
	}
}

func TestChecker_CheckNow_OlderVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		manifest := VersionManifest{
			Version: testManifestVersion,
			URL:     testManifestURL,
			SHA256:  testManifestSHA256,
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "2.0.0")

	result, err := c.CheckNow(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UpdateAvailable {
		t.Error("expected no update available when current is newer")
	}
}

func TestChecker_CheckNow_InvalidManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0")

	_, err := c.CheckNow(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestChecker_CheckNow_MissingFields(t *testing.T) {
	tests := []struct {
		name     string
		manifest VersionManifest
	}{
		{"missing version", VersionManifest{URL: "http://x", SHA256: "abc"}},
		{"missing url", VersionManifest{Version: testManifestVersion, SHA256: "abc"}},
		{"missing sha256", VersionManifest{Version: testManifestVersion, URL: "http://x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tt.manifest)
			}))
			defer server.Close()

			c := NewChecker(server.URL, "1.0.0")

			_, err := c.CheckNow(context.Background())
			if err == nil {
				t.Error("expected error for missing field")
			}
		})
	}
}

func TestChecker_CheckNow_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("intentional panic to close connection")
	}))
	server.Close()

	c := NewChecker(server.URL, "1.0.0")

	_, err := c.CheckNow(context.Background())
	if err == nil {
		t.Error("expected error for network failure")
	}
}

func TestChecker_CheckNow_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0")

	_, err := c.CheckNow(context.Background())
	if err == nil {
		t.Error("expected error for non-OK status")
	}
}

func TestChecker_PeriodicChecking(t *testing.T) {
	var checkCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		checkCount.Add(1)
		manifest := VersionManifest{
			Version: testManifestVersion,
			URL:     testManifestURL,
			SHA256:  testManifestSHA256,
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0",
		WithCheckInterval(50*time.Millisecond),
	)

	ctx := t.Context()

	c.Start(ctx)

	time.Sleep(50 * time.Millisecond)

	if !c.IsRunning() {
		t.Error("expected checker to be running")
	}

	c.Stop()

	if c.IsRunning() {
		t.Error("expected checker to be stopped")
	}
}

func TestChecker_GracefulShutdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		manifest := VersionManifest{
			Version: testManifestVersion,
			URL:     testManifestURL,
			SHA256:  testManifestSHA256,
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0",
		WithCheckInterval(1*time.Second),
	)

	ctx := context.Background()
	c.Start(ctx)

	if !c.IsRunning() {
		t.Error("expected checker to be running")
	}

	c.Stop()

	if c.IsRunning() {
		t.Error("expected checker to not be running after Stop")
	}

	c.Stop()
}

func TestChecker_DoubleStartIsSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		manifest := VersionManifest{
			Version: testManifestVersion,
			URL:     testManifestURL,
			SHA256:  testManifestSHA256,
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0",
		WithCheckInterval(1*time.Second),
	)

	ctx := context.Background()
	c.Start(ctx)
	c.Start(ctx)

	c.Stop()

	if c.IsRunning() {
		t.Error("expected checker to not be running after Stop")
	}
}

func TestChecker_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		manifest := VersionManifest{
			Version: testManifestVersion,
			URL:     testManifestURL,
			SHA256:  testManifestSHA256,
		}
		_ = json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	c := NewChecker(server.URL, "1.0.0",
		WithCheckInterval(1*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	cancel()

	time.Sleep(50 * time.Millisecond)

	c.Stop()

	if c.IsRunning() {
		t.Error("expected checker to not be running after context cancel")
	}
}

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.0.0", testV100},
		{"v1.0.0", "v1.0.0"},
		{"", "v0.0.0"},
		{"2.3.4", testNewVersion2},
		{"v2.3.4", testNewVersion2},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeVersion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

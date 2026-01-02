package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstaller_New(t *testing.T) {
	i := NewInstaller()

	if i.httpClient == nil {
		t.Error("expected HTTP client to be set")
	}

	if i.logger == nil {
		t.Error("expected logger to be set")
	}
}

func TestInstaller_NewWithOptions(t *testing.T) {
	customClient := &http.Client{}
	var progressCalled bool

	i := NewInstaller(
		WithInstallerHTTPClient(customClient),
		WithBinaryPath("/custom/path"),
		WithProgressCallback(func(downloaded, total int64) {
			progressCalled = true
		}),
	)

	if i.httpClient != customClient {
		t.Error("expected custom HTTP client")
	}

	if i.binaryPath != "/custom/path" {
		t.Errorf("expected binary path '/custom/path', got %q", i.binaryPath)
	}

	// Test progress callback is set
	if i.onProgress != nil {
		i.onProgress(100, 1000)
		if !progressCalled {
			t.Error("expected progress callback to be called")
		}
	}
}

func TestInstaller_DownloadAndVerify_Success(t *testing.T) {
	// Create test binary content
	binaryContent := []byte("#!/bin/bash\necho 'Hello World'")
	hash := sha256.Sum256(binaryContent)
	hashHex := hex.EncodeToString(hash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(binaryContent)
	}))
	defer server.Close()

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  hashHex,
	}

	i := NewInstaller()

	tmpPath, err := i.DownloadAndVerify(context.Background(), manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(tmpPath)

	// Verify file exists and has correct content
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}

	if string(content) != string(binaryContent) {
		t.Error("file content mismatch")
	}

	// Verify file is executable
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("failed to stat temp file: %v", err)
	}

	if info.Mode().Perm()&0100 == 0 {
		t.Error("expected file to be executable")
	}
}

func TestInstaller_DownloadAndVerify_BadChecksum(t *testing.T) {
	binaryContent := []byte("some binary content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(binaryContent)
	}))
	defer server.Close()

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  "wrongchecksum",
	}

	i := NewInstaller()

	_, err := i.DownloadAndVerify(context.Background(), manifest)
	if err == nil {
		t.Fatal("expected checksum error")
	}

	if !isChecksumMismatch(err) {
		t.Errorf("expected ErrChecksumMismatch, got: %v", err)
	}
}

func TestInstaller_DownloadAndVerify_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close() // Close immediately to cause network error

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  "abc123",
	}

	i := NewInstaller()

	_, err := i.DownloadAndVerify(context.Background(), manifest)
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestInstaller_DownloadAndVerify_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  "abc123",
	}

	i := NewInstaller()

	_, err := i.DownloadAndVerify(context.Background(), manifest)
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}

	if !isDownloadFailed(err) {
		t.Errorf("expected ErrDownloadFailed, got: %v", err)
	}
}

func TestInstaller_DownloadAndVerify_ProgressCallback(t *testing.T) {
	binaryContent := []byte("binary content for progress test")
	hash := sha256.Sum256(binaryContent)
	hashHex := hex.EncodeToString(hash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(binaryContent)))
		w.WriteHeader(http.StatusOK)
		w.Write(binaryContent)
	}))
	defer server.Close()

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  hashHex,
	}

	var progressCalls int
	var lastDownloaded int64

	i := NewInstaller(
		WithProgressCallback(func(downloaded, total int64) {
			progressCalls++
			lastDownloaded = downloaded
		}),
	)

	tmpPath, err := i.DownloadAndVerify(context.Background(), manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(tmpPath)

	if progressCalls == 0 {
		t.Error("expected progress callback to be called")
	}

	if lastDownloaded != int64(len(binaryContent)) {
		t.Errorf("expected lastDownloaded %d, got %d", len(binaryContent), lastDownloaded)
	}
}

func TestInstaller_DownloadAndVerify_LargeFile(t *testing.T) {
	// Create a larger binary (1MB)
	binaryContent := make([]byte, 1024*1024)
	for i := range binaryContent {
		binaryContent[i] = byte(i % 256)
	}
	hash := sha256.Sum256(binaryContent)
	hashHex := hex.EncodeToString(hash[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(binaryContent)
	}))
	defer server.Close()

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  hashHex,
	}

	i := NewInstaller()

	tmpPath, err := i.DownloadAndVerify(context.Background(), manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer os.Remove(tmpPath)

	// Verify file size
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("failed to stat temp file: %v", err)
	}

	if info.Size() != int64(len(binaryContent)) {
		t.Errorf("expected file size %d, got %d", len(binaryContent), info.Size())
	}
}

func TestInstaller_AtomicReplace(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "installer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source file
	srcPath := filepath.Join(tmpDir, "source")
	srcContent := []byte("new binary content")
	if err := os.WriteFile(srcPath, srcContent, 0755); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	// Create destination file
	dstPath := filepath.Join(tmpDir, "destination")
	dstContent := []byte("old binary content")
	if err := os.WriteFile(dstPath, dstContent, 0755); err != nil {
		t.Fatalf("failed to write destination file: %v", err)
	}

	i := NewInstaller()

	if err := i.atomicReplace(srcPath, dstPath); err != nil {
		t.Fatalf("atomicReplace failed: %v", err)
	}

	// Verify destination has new content
	content, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}

	if string(content) != string(srcContent) {
		t.Errorf("expected content %q, got %q", srcContent, content)
	}
}

func TestInstaller_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		select {
		case <-r.Context().Done():
			return
		case <-make(chan struct{}): // Block forever
		}
	}))
	defer server.Close()

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  "abc123",
	}

	i := NewInstaller()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := i.DownloadAndVerify(ctx, manifest)
	if err == nil {
		t.Fatal("expected error due to cancelled context")
	}
}

// Helper functions to check error types
func isChecksumMismatch(err error) bool {
	return err != nil && (err == ErrChecksumMismatch ||
		(len(err.Error()) > 0 && err.Error()[:len("checksum mismatch")] == "checksum mismatch"))
}

func isDownloadFailed(err error) bool {
	return err != nil && (err == ErrDownloadFailed ||
		(len(err.Error()) > 0 && err.Error()[:len("download failed")] == "download failed"))
}

func TestInstaller_Install(t *testing.T) {
	// Create temp dir for the "current binary"
	tmpDir, err := os.MkdirTemp("", "installer-install-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a "current binary"
	currentBinary := filepath.Join(tmpDir, "app")
	if err := os.WriteFile(currentBinary, []byte("old version"), 0755); err != nil {
		t.Fatalf("failed to create current binary: %v", err)
	}

	// Prepare new binary content
	newContent := []byte("new version")
	hash := sha256.Sum256(newContent)
	hashHex := hex.EncodeToString(hash[:])

	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(newContent)
	}))
	defer server.Close()

	manifest := &VersionManifest{
		Version: "2.0.0",
		URL:     server.URL,
		SHA256:  hashHex,
	}

	// Create installer targeting our temp binary
	i := NewInstaller(
		WithBinaryPath(currentBinary),
	)

	// Perform install
	if err := i.Install(context.Background(), manifest); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Verify the binary was replaced
	content, err := os.ReadFile(currentBinary)
	if err != nil {
		t.Fatalf("failed to read binary after install: %v", err)
	}

	if string(content) != string(newContent) {
		t.Errorf("content mismatch after install. allow: %q, got: %q", newContent, content)
	}
}

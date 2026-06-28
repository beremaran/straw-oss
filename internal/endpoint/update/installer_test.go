package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

	if i.onProgress != nil {
		i.onProgress(100, 1000)
		if !progressCalled {
			t.Error("expected progress callback to be called")
		}
	}
}

func TestInstaller_DownloadAndVerify_Success(t *testing.T) {
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

	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}

	if string(content) != string(binaryContent) {
		t.Error("file content mismatch")
	}

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
	server.Close()

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

	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("failed to stat temp file: %v", err)
	}

	if info.Size() != int64(len(binaryContent)) {
		t.Errorf("expected file size %d, got %d", len(binaryContent), info.Size())
	}
}

func TestInstaller_AtomicReplace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "installer-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source")
	srcContent := []byte("new binary content")
	if err := os.WriteFile(srcPath, srcContent, 0755); err != nil {
		t.Fatalf("failed to write source file: %v", err)
	}

	dstPath := filepath.Join(tmpDir, "destination")
	dstContent := []byte("old binary content")
	if err := os.WriteFile(dstPath, dstContent, 0755); err != nil {
		t.Fatalf("failed to write destination file: %v", err)
	}

	i := NewInstaller()

	if err := i.atomicReplace(srcPath, dstPath); err != nil {
		t.Fatalf("atomicReplace failed: %v", err)
	}

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
		select {
		case <-r.Context().Done():
			return
		case <-make(chan struct{}):
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
	cancel()

	_, err := i.DownloadAndVerify(ctx, manifest)
	if err == nil {
		t.Fatal("expected error due to cancelled context")
	}
}

func isChecksumMismatch(err error) bool {
	return err != nil && (errors.Is(err, ErrChecksumMismatch) ||
		(len(err.Error()) > 0 && err.Error()[:len("checksum mismatch")] == "checksum mismatch"))
}

func isDownloadFailed(err error) bool {
	return err != nil && (errors.Is(err, ErrDownloadFailed) ||
		(len(err.Error()) > 0 && err.Error()[:len("download failed")] == "download failed"))
}

func TestInstaller_Install(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "installer-install-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tmpDir)

	currentBinary := filepath.Join(tmpDir, "app")
	if err := os.WriteFile(currentBinary, []byte("old version"), 0755); err != nil {
		t.Fatalf("failed to create current binary: %v", err)
	}

	newContent := []byte("new version")
	hash := sha256.Sum256(newContent)
	hashHex := hex.EncodeToString(hash[:])

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

	i := NewInstaller(
		WithBinaryPath(currentBinary),
	)

	if err := i.Install(context.Background(), manifest); err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	content, err := os.ReadFile(currentBinary)
	if err != nil {
		t.Fatalf("failed to read binary after install: %v", err)
	}

	if string(content) != string(newContent) {
		t.Errorf("content mismatch after install. allow: %q, got: %q", newContent, content)
	}
}

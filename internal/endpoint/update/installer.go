// Package update provides self-update functionality for the Endpoint worker.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// Common errors returned by the Installer.
var (
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrDownloadFailed   = errors.New("download failed")
	ErrInstallFailed    = errors.New("installation failed")
)

// Installer handles downloading and installing binary updates.
type Installer struct {
	httpClient *http.Client
	logger     *slog.Logger
	binaryPath string // path to current binary (auto-detected if empty)

	// Callbacks for progress reporting
	onProgress func(downloaded, total int64)
}

// InstallerOption is a functional option for configuring the Installer.
type InstallerOption func(*Installer)

// WithInstallerLogger sets the logger for the installer.
func WithInstallerLogger(logger *slog.Logger) InstallerOption {
	return func(i *Installer) {
		i.logger = logger
	}
}

// WithInstallerHTTPClient sets a custom HTTP client for downloads.
func WithInstallerHTTPClient(client *http.Client) InstallerOption {
	return func(i *Installer) {
		i.httpClient = client
	}
}

// WithBinaryPath sets the path to the binary to replace.
// If not set, the current executable path is used.
func WithBinaryPath(path string) InstallerOption {
	return func(i *Installer) {
		i.binaryPath = path
	}
}

// WithProgressCallback sets a callback for download progress reporting.
func WithProgressCallback(cb func(downloaded, total int64)) InstallerOption {
	return func(i *Installer) {
		i.onProgress = cb
	}
}

// NewInstaller creates a new Installer.
func NewInstaller(opts ...InstallerOption) *Installer {
	i := &Installer{
		httpClient: &http.Client{
			Timeout: 0, // no timeout for downloads (may be large)
		},
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(i)
	}

	return i
}

// Install downloads the new binary, verifies its checksum, and replaces the current binary.
// This method does NOT restart the process - call ReplaceAndRestart for that.
func (i *Installer) Install(ctx context.Context, manifest *VersionManifest) error {
	i.logger.Info("starting update installation",
		"version", manifest.Version,
		"url", manifest.URL,
	)

	tmpPath, err := i.DownloadAndVerify(ctx, manifest)
	if err != nil {
		return err
	}

	// Get current binary path
	binaryPath := i.binaryPath
	if binaryPath == "" {
		binaryPath, err = os.Executable()
		if err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("%w: failed to get executable path: %v", ErrInstallFailed, err)
		}
		// Resolve symlinks
		binaryPath, err = filepath.EvalSymlinks(binaryPath)
		if err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("%w: failed to resolve symlinks: %v", ErrInstallFailed, err)
		}
	}

	// Perform atomic replacement
	if err := i.atomicReplace(tmpPath, binaryPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	i.logger.Info("update installed successfully",
		"version", manifest.Version,
		"path", binaryPath,
	)

	return nil
}

// DownloadAndVerify downloads the binary and verifies its SHA256 checksum.
// Returns the path to the temporary file containing the verified binary.
// The caller is responsible for removing the temporary file after use.
func (i *Installer) DownloadAndVerify(ctx context.Context, manifest *VersionManifest) (string, error) {
	i.logger.Debug("downloading update",
		"url", manifest.URL,
		"expected_sha256", manifest.SHA256,
	)

	// Create temp file
	tmpFile, err := os.CreateTemp("", "straw-update-*")
	if err != nil {
		return "", fmt.Errorf("%w: failed to create temp file: %v", ErrDownloadFailed, err)
	}
	tmpPath := tmpFile.Name()

	// Ensure cleanup on error
	success := false
	defer func() {
		_ = tmpFile.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	// Download the binary
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.URL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create request: %v", ErrDownloadFailed, err)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: HTTP request failed: %v", ErrDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: unexpected status code: %d", ErrDownloadFailed, resp.StatusCode)
	}

	// Calculate checksum while downloading
	hasher := sha256.New()
	var downloaded int64

	// Wrap with progress reporting if callback is set
	var reader io.Reader = resp.Body
	if i.onProgress != nil {
		reader = &progressReader{
			reader:     resp.Body,
			total:      resp.ContentLength,
			onProgress: i.onProgress,
		}
	}

	// Stream to file while calculating hash
	multiWriter := io.MultiWriter(tmpFile, hasher)
	downloaded, err = io.Copy(multiWriter, reader)
	if err != nil {
		return "", fmt.Errorf("%w: failed to download: %v", ErrDownloadFailed, err)
	}

	i.logger.Debug("download complete", "bytes", downloaded)

	// Verify checksum
	actualSum := hex.EncodeToString(hasher.Sum(nil))
	expectedSum := manifest.SHA256

	if actualSum != expectedSum {
		return "", fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedSum, actualSum)
	}

	i.logger.Debug("checksum verified", "sha256", actualSum)

	// Make executable
	if err := tmpFile.Chmod(0755); err != nil {
		return "", fmt.Errorf("%w: failed to set permissions: %v", ErrInstallFailed, err)
	}

	success = true
	return tmpPath, nil
}

// atomicReplace performs an atomic replacement of the target binary.
func (i *Installer) atomicReplace(srcPath, dstPath string) error {
	// On Unix systems, we can rename directly (atomic operation)
	// On Windows, we need to handle the case where the running binary is locked

	if runtime.GOOS == "windows" {
		// Windows: rename current binary to .old, then rename new to current
		oldPath := dstPath + ".old"
		_ = os.Remove(oldPath) // Remove any existing .old file

		if err := os.Rename(dstPath, oldPath); err != nil {
			return fmt.Errorf("%w: failed to backup current binary: %v", ErrInstallFailed, err)
		}

		if err := os.Rename(srcPath, dstPath); err != nil {
			// Try to restore
			_ = os.Rename(oldPath, dstPath)
			return fmt.Errorf("%w: failed to install new binary: %v", ErrInstallFailed, err)
		}

		// Clean up old binary (may fail on Windows, that's ok)
		_ = os.Remove(oldPath)
	} else {
		// Unix: atomic rename
		if err := os.Rename(srcPath, dstPath); err != nil {
			return fmt.Errorf("%w: failed to install new binary: %v", ErrInstallFailed, err)
		}
	}

	return nil
}

// ReplaceAndRestart performs an exec syscall to restart with the new binary.
// This function does not return on success.
func (i *Installer) ReplaceAndRestart() error {
	binaryPath := i.binaryPath
	var err error
	if binaryPath == "" {
		binaryPath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
	}

	i.logger.Info("restarting with new binary", "path", binaryPath)

	// Get current args and environment
	args := os.Args
	env := os.Environ()

	// Exec the new binary
	return syscall.Exec(binaryPath, args, env)
}

// progressReader wraps an io.Reader to report download progress.
type progressReader struct {
	reader     io.Reader
	downloaded int64
	total      int64
	onProgress func(downloaded, total int64)
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.downloaded += int64(n)
	if pr.onProgress != nil {
		pr.onProgress(pr.downloaded, pr.total)
	}
	return n, err
}

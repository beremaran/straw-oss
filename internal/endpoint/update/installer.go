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

var (
	// ErrChecksumMismatch is returned when the downloaded file SHA256 does not match the manifest.
	ErrChecksumMismatch = errors.New("checksum mismatch")
	// ErrDownloadFailed is returned when the HTTP download fails.
	ErrDownloadFailed = errors.New("download failed")
	// ErrInstallFailed is returned when the binary replacement or restart fails.
	ErrInstallFailed = errors.New("installation failed")
	// ErrBinaryPathNotAbsolute is returned when the binary path is not absolute.
	ErrBinaryPathNotAbsolute = errors.New("binary path must be absolute")
)

const binaryPerms = 0o755

// Installer downloads a new binary from a manifest and atomically replaces the running executable.
type Installer struct {
	httpClient *http.Client
	logger     *slog.Logger
	binaryPath string
	onProgress func(downloaded, total int64)
}

// InstallerOption configures an Installer.
type InstallerOption func(*Installer)

// WithInstallerLogger sets the logger used by the installer.
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

// WithBinaryPath sets the path of the binary to replace.
func WithBinaryPath(path string) InstallerOption {
	return func(i *Installer) {
		i.binaryPath = path
	}
}

// WithProgressCallback sets a callback invoked during downloads with (downloaded, total) bytes.
func WithProgressCallback(cb func(downloaded, total int64)) InstallerOption {
	return func(i *Installer) {
		i.onProgress = cb
	}
}

// NewInstaller creates an Installer with default settings.
func NewInstaller(opts ...InstallerOption) *Installer {
	i := &Installer{
		httpClient: &http.Client{
			Timeout: 0,
		},
		logger: slog.Default(),
	}

	for _, opt := range opts {
		opt(i)
	}

	return i
}

// Install downloads the manifest version and atomically replaces the current binary.
func (i *Installer) Install(ctx context.Context, manifest *VersionManifest) error {
	i.logger.Info("starting update installation",
		"version", manifest.Version,
		"url", manifest.URL,
	)

	tmpPath, err := i.DownloadAndVerify(ctx, manifest)
	if err != nil {
		return err
	}

	binaryPath := i.binaryPath
	if binaryPath == "" {
		binaryPath, err = os.Executable()
		if err != nil {
			_ = os.Remove(tmpPath)

			return fmt.Errorf("%w: failed to get executable path: %w", ErrInstallFailed, err)
		}

		binaryPath, err = filepath.EvalSymlinks(binaryPath)
		if err != nil {
			_ = os.Remove(tmpPath)

			return fmt.Errorf("%w: failed to resolve symlinks: %w", ErrInstallFailed, err)
		}
	}

	err = i.atomicReplace(tmpPath, binaryPath)
	if err != nil {
		_ = os.Remove(tmpPath)

		return err
	}

	i.logger.Info("update installed successfully",
		"version", manifest.Version,
		"path", binaryPath,
	)

	return nil
}

// DownloadAndVerify downloads the manifest version, verifies its checksum, and returns the temp path.
func (i *Installer) DownloadAndVerify(ctx context.Context, manifest *VersionManifest) (string, error) {
	i.logger.Debug("downloading update",
		"url", manifest.URL,
		"expected_sha256", manifest.SHA256,
	)

	tmpFile, tmpPath, err := createUpdateTempFile()
	if err != nil {
		return "", err
	}

	success := false

	defer func() {
		_ = tmpFile.Close()

		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	resp, err := download(ctx, i.httpClient, manifest.URL)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	hasher := sha256.New()

	downloaded, err := copyDownload(tmpFile, hasher, resp, i.onProgress)
	if err != nil {
		return "", fmt.Errorf("%w: failed to download: %w", ErrDownloadFailed, err)
	}

	i.logger.Debug("download complete", "bytes", downloaded)

	actualSum, err := verifyChecksum(hasher.Sum(nil), manifest.SHA256)
	if err != nil {
		return "", err
	}

	i.logger.Debug("checksum verified", "sha256", actualSum)

	err = tmpFile.Chmod(binaryPerms)
	if err != nil {
		return "", fmt.Errorf("%w: failed to set permissions: %w", ErrInstallFailed, err)
	}

	success = true

	return tmpPath, nil
}

func createUpdateTempFile() (*os.File, string, error) {
	tmpFile, err := os.CreateTemp("", "straw-update-*")
	if err != nil {
		return nil, "", fmt.Errorf("%w: failed to create temp file: %w", ErrDownloadFailed, err)
	}

	return tmpFile, tmpFile.Name(), nil
}

func download(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %w", ErrDownloadFailed, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: HTTP request failed: %w", ErrDownloadFailed, err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("%w: unexpected status code: %d", ErrDownloadFailed, resp.StatusCode)
	}

	return resp, nil
}

func copyDownload(
	tmpFile *os.File,
	hasher io.Writer,
	resp *http.Response,
	onProgress func(downloaded, total int64),
) (int64, error) {
	reader := downloadReader(resp, onProgress)
	multiWriter := io.MultiWriter(tmpFile, hasher)

	n, err := io.Copy(multiWriter, reader)
	if err != nil {
		return n, fmt.Errorf("copy failed: %w", err)
	}

	return n, nil
}

func downloadReader(resp *http.Response, onProgress func(downloaded, total int64)) io.Reader {
	if onProgress == nil {
		return resp.Body
	}

	return &progressReader{
		reader:     resp.Body,
		total:      resp.ContentLength,
		onProgress: onProgress,
	}
}

func verifyChecksum(actual []byte, expected string) (string, error) {
	actualSum := hex.EncodeToString(actual)
	if actualSum != expected {
		return "", fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, actualSum)
	}

	return actualSum, nil
}

// ReplaceAndRestart replaces the current process with the new binary via exec.
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

	if !filepath.IsAbs(binaryPath) {
		return ErrBinaryPathNotAbsolute
	}

	_, statErr := os.Stat(binaryPath)
	if statErr != nil {
		return fmt.Errorf("binary not found: %w", statErr)
	}

	args := os.Args
	env := os.Environ()

	//nolint:gosec // binaryPath validated: absolute path + os.Stat check
	exErr := syscall.Exec(binaryPath, args, env)
	if exErr != nil {
		return fmt.Errorf("exec failed: %w", exErr)
	}

	return nil
}

func (i *Installer) atomicReplace(srcPath, dstPath string) error {
	if runtime.GOOS == "windows" {
		oldPath := dstPath + ".old"
		_ = os.Remove(oldPath)

		err := os.Rename(dstPath, oldPath)
		if err != nil {
			return fmt.Errorf("%w: failed to backup current binary: %w", ErrInstallFailed, err)
		}

		err = os.Rename(srcPath, dstPath)
		if err != nil {
			_ = os.Rename(oldPath, dstPath)

			return fmt.Errorf("%w: failed to install new binary: %w", ErrInstallFailed, err)
		}

		_ = os.Remove(oldPath)
	} else {
		err := os.Rename(srcPath, dstPath)
		if err != nil {
			return fmt.Errorf("%w: failed to install new binary: %w", ErrInstallFailed, err)
		}
	}

	return nil
}

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

	if err != nil {
		if errors.Is(err, io.EOF) {
			return n, io.EOF
		}

		return n, fmt.Errorf("read failed: %w", err)
	}

	return n, nil
}

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
	ErrChecksumMismatch = errors.New("checksum mismatch")
	ErrDownloadFailed   = errors.New("download failed")
	ErrInstallFailed    = errors.New("installation failed")
)

type Installer struct {
	httpClient *http.Client
	logger     *slog.Logger
	binaryPath string

	onProgress func(downloaded, total int64)
}

type InstallerOption func(*Installer)

func WithInstallerLogger(logger *slog.Logger) InstallerOption {
	return func(i *Installer) {
		i.logger = logger
	}
}

func WithInstallerHTTPClient(client *http.Client) InstallerOption {
	return func(i *Installer) {
		i.httpClient = client
	}
}

func WithBinaryPath(path string) InstallerOption {
	return func(i *Installer) {
		i.binaryPath = path
	}
}

func WithProgressCallback(cb func(downloaded, total int64)) InstallerOption {
	return func(i *Installer) {
		i.onProgress = cb
	}
}

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

func (i *Installer) DownloadAndVerify(ctx context.Context, manifest *VersionManifest) (string, error) {
	i.logger.Debug("downloading update",
		"url", manifest.URL,
		"expected_sha256", manifest.SHA256,
	)

	tmpFile, err := os.CreateTemp("", "straw-update-*")
	if err != nil {
		return "", fmt.Errorf("%w: failed to create temp file: %w", ErrDownloadFailed, err)
	}
	tmpPath := tmpFile.Name()

	success := false
	defer func() {
		_ = tmpFile.Close()
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifest.URL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: failed to create request: %w", ErrDownloadFailed, err)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: HTTP request failed: %w", ErrDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: unexpected status code: %d", ErrDownloadFailed, resp.StatusCode)
	}

	hasher := sha256.New()
	var downloaded int64

	var reader io.Reader = resp.Body
	if i.onProgress != nil {
		reader = &progressReader{
			reader:     resp.Body,
			total:      resp.ContentLength,
			onProgress: i.onProgress,
		}
	}

	multiWriter := io.MultiWriter(tmpFile, hasher)
	downloaded, err = io.Copy(multiWriter, reader)
	if err != nil {
		return "", fmt.Errorf("%w: failed to download: %w", ErrDownloadFailed, err)
	}

	i.logger.Debug("download complete", "bytes", downloaded)

	actualSum := hex.EncodeToString(hasher.Sum(nil))
	expectedSum := manifest.SHA256

	if actualSum != expectedSum {
		return "", fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expectedSum, actualSum)
	}

	i.logger.Debug("checksum verified", "sha256", actualSum)

	if err := tmpFile.Chmod(0755); err != nil {
		return "", fmt.Errorf("%w: failed to set permissions: %w", ErrInstallFailed, err)
	}

	success = true

	return tmpPath, nil
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

	args := os.Args
	env := os.Environ()

	return syscall.Exec(binaryPath, args, env)
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

	return n, err
}

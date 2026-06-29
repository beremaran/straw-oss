// Package update provides automatic update checking and installation for the endpoint binary.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

var (
	// ErrUnexpectedStatusCode is returned when the update server responds with a non-200 status.
	ErrUnexpectedStatusCode = errors.New("unexpected status code")
	// ErrManifestMissingVersion is returned when the manifest lacks a version field.
	ErrManifestMissingVersion = errors.New("manifest missing version field")
	// ErrManifestMissingURL is returned when the manifest lacks a URL field.
	ErrManifestMissingURL = errors.New("manifest missing url field")
	// ErrManifestMissingSHA256 is returned when the manifest lacks a SHA256 field.
	ErrManifestMissingSHA256 = errors.New("manifest missing sha256 field")
)

const (
	initialCheckDelay = 10 * time.Second
	maxManifestSize   = 1 << 20
)

// Checker periodically checks for new versions of the endpoint binary.
type Checker struct {
	updateURL      string
	currentVersion string
	interval       time.Duration
	httpTimeout    time.Duration
	logger         *slog.Logger
	callback       Callback
	httpClient     *http.Client
	mu             sync.Mutex
	running        bool
	cancel         context.CancelFunc
	done           chan struct{}
}

// CheckerOption configures a Checker.
type CheckerOption func(*Checker)

// WithCheckInterval sets how often the checker polls for updates.
func WithCheckInterval(d time.Duration) CheckerOption {
	return func(c *Checker) {
		if d > 0 {
			c.interval = d
		}
	}
}

// WithHTTPTimeout sets the timeout for HTTP requests made by the checker.
func WithHTTPTimeout(d time.Duration) CheckerOption {
	return func(c *Checker) {
		if d > 0 {
			c.httpTimeout = d
		}
	}
}

// WithCheckerLogger sets the logger used by the checker.
func WithCheckerLogger(logger *slog.Logger) CheckerOption {
	return func(c *Checker) {
		c.logger = logger
	}
}

// WithUpdateCallback sets a callback invoked when an update is available;
// if it returns false the installation is skipped.
func WithUpdateCallback(cb Callback) CheckerOption {
	return func(c *Checker) {
		c.callback = cb
	}
}

// WithHTTPClient sets a custom HTTP client for the checker.
func WithHTTPClient(client *http.Client) CheckerOption {
	return func(c *Checker) {
		c.httpClient = client
	}
}

// NewChecker creates a Checker that polls updateURL for a JSON manifest.
func NewChecker(updateURL, currentVersion string, opts ...CheckerOption) *Checker {
	c := &Checker{
		updateURL:      updateURL,
		currentVersion: normalizeVersion(currentVersion),
		interval:       DefaultCheckInterval,
		httpTimeout:    DefaultHTTPTimeout,
		logger:         slog.Default(),
		callback:       func(*Result) bool { return true },
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: c.httpTimeout,
		}
	}

	return c
}

// Start begins periodic update checks in a background goroutine.
func (c *Checker) Start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		c.logger.Warn("update checker already running")

		return
	}

	ctx, c.cancel = context.WithCancel(ctx)
	c.done = make(chan struct{})
	c.running = true

	c.logger.Info("starting update checker",
		"update_url", c.updateURL,
		"current_version", c.currentVersion,
		"interval", c.interval,
	)

	go c.run(ctx)
}

// Stop signals the checker to halt and waits for the goroutine to exit.
func (c *Checker) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()

		return
	}

	cancel := c.cancel
	done := c.done
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if done != nil {
		<-done
	}

	c.mu.Lock()
	c.running = false
	c.mu.Unlock()

	c.logger.Info("update checker stopped")
}

// CheckNow performs a single update check against the update server.
func (c *Checker) CheckNow(ctx context.Context) (*Result, error) {
	manifest, err := c.fetchManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}

	newVersion := normalizeVersion(manifest.Version)
	updateAvailable := semver.Compare(newVersion, c.currentVersion) > 0

	return &Result{
		UpdateAvailable: updateAvailable,
		CurrentVersion:  c.currentVersion,
		NewVersion:      manifest.Version,
		DownloadURL:     manifest.URL,
		Checksum:        manifest.SHA256,
	}, nil
}

// IsRunning reports whether the checker goroutine is active.
func (c *Checker) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.running
}

func (c *Checker) run(ctx context.Context) {
	defer close(c.done)

	select {
	case <-ctx.Done():
		return
	case <-time.After(initialCheckDelay):
		c.performCheck(ctx)
	}

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.performCheck(ctx)
		}
	}
}

func (c *Checker) performCheck(ctx context.Context) {
	c.logger.Debug("checking for updates", "url", c.updateURL)

	result, err := c.CheckNow(ctx)
	if err != nil {
		c.logger.Error("update check failed", "error", err)

		return
	}

	if !result.UpdateAvailable {
		c.logger.Debug("no update available",
			"current_version", result.CurrentVersion,
			"latest_version", result.NewVersion,
		)

		return
	}

	c.logger.Info("update available",
		"current_version", result.CurrentVersion,
		"new_version", result.NewVersion,
		"download_url", result.DownloadURL,
	)

	if c.callback != nil && c.callback(result) {
		c.logger.Info("update approved by callback, proceeding with installation")
	}
}

func (c *Checker) fetchManifest(ctx context.Context) (*VersionManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.updateURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "StrawProxy-Endpoint/"+c.currentVersion)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d", ErrUnexpectedStatusCode, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var manifest VersionManifest

	err = json.Unmarshal(body, &manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	if manifest.Version == "" {
		return nil, ErrManifestMissingVersion
	}

	if manifest.URL == "" {
		return nil, ErrManifestMissingURL
	}

	if manifest.SHA256 == "" {
		return nil, ErrManifestMissingSHA256
	}

	return &manifest, nil
}

func normalizeVersion(version string) string {
	if version == "" {
		return "v0.0.0"
	}

	if version[0] != 'v' {
		return "v" + version
	}

	return version
}

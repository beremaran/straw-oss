// Package update provides self-update functionality for the Endpoint worker.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

// Checker periodically checks for updates and notifies when a new version is available.
type Checker struct {
	updateURL      string
	currentVersion string
	interval       time.Duration
	httpTimeout    time.Duration
	logger         *slog.Logger
	callback       Callback
	httpClient     *http.Client

	// Lifecycle management
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// CheckerOption is a functional option for configuring the Checker.
type CheckerOption func(*Checker)

// WithCheckInterval sets the interval between update checks.
func WithCheckInterval(d time.Duration) CheckerOption {
	return func(c *Checker) {
		if d > 0 {
			c.interval = d
		}
	}
}

// WithHTTPTimeout sets the HTTP timeout for update checks.
func WithHTTPTimeout(d time.Duration) CheckerOption {
	return func(c *Checker) {
		if d > 0 {
			c.httpTimeout = d
		}
	}
}

// WithCheckerLogger sets the logger for the checker.
func WithCheckerLogger(logger *slog.Logger) CheckerOption {
	return func(c *Checker) {
		c.logger = logger
	}
}

// WithUpdateCallback sets the callback for when an update is available.
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

// NewChecker creates a new update Checker.
// The currentVersion should be a semantic version string (e.g., "1.2.3" or "v1.2.3").
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

	// Create HTTP client if not provided
	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Timeout: c.httpTimeout,
		}
	}

	return c
}

// Start begins periodic update checking.
// This method returns immediately and runs checks in a goroutine.
// Call Stop() to stop checking.
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

// Stop gracefully stops the update checker.
// It blocks until the checker has fully stopped.
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

// CheckNow performs an immediate update check.
// This can be called independently of the periodic checking.
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

// IsRunning returns true if the checker is currently running.
func (c *Checker) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// run is the main checking loop.
func (c *Checker) run(ctx context.Context) {
	defer close(c.done)

	// Perform initial check after a short delay (avoid startup thundering herd)
	select {
	case <-ctx.Done():
		return
	case <-time.After(10 * time.Second):
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

// performCheck performs a single update check and invokes callback if update available.
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

	// Invoke callback to determine if update should be applied
	if c.callback != nil && c.callback(result) {
		c.logger.Info("update approved by callback, proceeding with installation")
		// Note: actual installation is handled by the callback or Installer
	}
}

// fetchManifest fetches and parses the version manifest from the update URL.
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
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Limit response size to prevent memory exhaustion
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var manifest VersionManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate required fields
	if manifest.Version == "" {
		return nil, fmt.Errorf("manifest missing version field")
	}
	if manifest.URL == "" {
		return nil, fmt.Errorf("manifest missing url field")
	}
	if manifest.SHA256 == "" {
		return nil, fmt.Errorf("manifest missing sha256 field")
	}

	return &manifest, nil
}

// normalizeVersion ensures the version has a "v" prefix for semver comparison.
func normalizeVersion(version string) string {
	if version == "" {
		return "v0.0.0"
	}
	if version[0] != 'v' {
		return "v" + version
	}
	return version
}

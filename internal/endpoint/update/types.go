// Package update provides self-update functionality for the Endpoint worker.
// Endpoints periodically check for new versions and can auto-update themselves.
package update

import "time"

// DefaultCheckInterval is the default interval between update checks.
const DefaultCheckInterval = 5 * time.Minute

// DefaultHTTPTimeout is the default timeout for HTTP requests during update checks.
const DefaultHTTPTimeout = 30 * time.Second

// VersionManifest is the expected JSON response from the update URL.
// Example:
//
//	{
//	  "version": "1.2.3",
//	  "url": "https://updates.example.com/endpoint-v1.2.3",
//	  "sha256": "abc123..."
//	}
type VersionManifest struct {
	// Version is the semantic version of the available binary (e.g., "1.2.3").
	Version string `json:"version"`

	// URL is the download URL for the new binary.
	URL string `json:"url"`

	// SHA256 is the hex-encoded SHA256 checksum of the binary.
	SHA256 string `json:"sha256"`
}

// UpdateResult contains the result of an update check.
type UpdateResult struct {
	// UpdateAvailable indicates whether a newer version is available.
	UpdateAvailable bool

	// CurrentVersion is the version currently running.
	CurrentVersion string

	// NewVersion is the version available for download (if any).
	NewVersion string

	// DownloadURL is the URL to download the new version from.
	DownloadURL string

	// Checksum is the expected SHA256 checksum of the new binary.
	Checksum string
}

// UpdateCallback is called when an update is available.
// The callback receives the update result and should return true if the
// update should be applied, or false to skip it.
type UpdateCallback func(result *UpdateResult) bool

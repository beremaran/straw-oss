package update

import "time"

// DefaultCheckInterval is how often the updater checks for new versions.
const DefaultCheckInterval = 5 * time.Minute

// DefaultHTTPTimeout is the default timeout for HTTP requests.
const DefaultHTTPTimeout = 30 * time.Second

// VersionManifest describes an available update release.
type VersionManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

// Result contains information about an update check.
type Result struct {
	UpdateAvailable bool
	CurrentVersion  string
	NewVersion      string
	DownloadURL     string
	Checksum        string
}

// Callback is invoked when an update is available; returning false skips installation.
type Callback func(result *Result) bool

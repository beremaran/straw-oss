package update

import "time"

const DefaultCheckInterval = 5 * time.Minute

const DefaultHTTPTimeout = 30 * time.Second

type VersionManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}

type Result struct {
	UpdateAvailable bool
	CurrentVersion  string
	NewVersion      string
	DownloadURL     string
	Checksum        string
}

type Callback func(result *Result) bool

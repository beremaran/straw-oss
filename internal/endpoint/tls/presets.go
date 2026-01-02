package tls

import (
	utls "github.com/refraction-networking/utls"
)

// Presets maps fingerprint preset IDs to utls ClientHelloID values.
// These presets allow the endpoint to mimic real browser TLS fingerprints.
//
// Available presets are based on utls v1.8.1. When adding new presets,
// verify they exist in the utls library version being used.
var Presets = map[string]utls.ClientHelloID{
	// Chrome presets
	"chrome-133": utls.HelloChrome_133, // Latest stable
	"chrome-131": utls.HelloChrome_131,
	"chrome-120": utls.HelloChrome_120,
	"chrome":     utls.HelloChrome_Auto, // Auto-select latest

	// Firefox presets
	"firefox-120": utls.HelloFirefox_120,
	"firefox":     utls.HelloFirefox_Auto, // Auto-select latest

	// Safari presets
	"safari-16": utls.HelloSafari_16_0,
	"safari":    utls.HelloSafari_Auto, // Auto-select latest

	// Edge presets
	"edge-106": utls.HelloEdge_106,
	"edge-85":  utls.HelloEdge_85,
	"edge":     utls.HelloEdge_Auto, // Auto-select latest

	// Generic/fallback presets
	"auto":       utls.HelloGolang,
	"randomized": utls.HelloRandomized,
}

// DefaultPreset is the fingerprint used when no specific preset is requested.
const DefaultPreset = "chrome-133"

// GetPreset returns the utls ClientHelloID for the given preset name.
// Returns false if the preset is not recognized.
func GetPreset(name string) (utls.ClientHelloID, bool) {
	preset, ok := Presets[name]
	return preset, ok
}

// ListPresets returns a slice of all available preset names.
func ListPresets() []string {
	names := make([]string, 0, len(Presets))
	for name := range Presets {
		names = append(names, name)
	}
	return names
}

// PresetInfo contains metadata about a fingerprint preset.
type PresetInfo struct {
	ID          string
	Description string
	Browser     string
	Version     string
	Deprecated  bool
}

// GetPresetInfo returns metadata about a fingerprint preset.
func GetPresetInfo(name string) (PresetInfo, bool) {
	info, ok := presetInfoMap[name]
	return info, ok
}

// presetInfoMap contains detailed metadata for each preset.
var presetInfoMap = map[string]PresetInfo{
	"chrome-133": {
		ID:          "chrome-133",
		Description: "Chrome 133 (current stable)",
		Browser:     "Chrome",
		Version:     "133",
		Deprecated:  false,
	},
	"chrome-131": {
		ID:          "chrome-131",
		Description: "Chrome 131",
		Browser:     "Chrome",
		Version:     "131",
		Deprecated:  false,
	},
	"chrome-120": {
		ID:          "chrome-120",
		Description: "Chrome 120 (older version)",
		Browser:     "Chrome",
		Version:     "120",
		Deprecated:  true,
	},
	"chrome": {
		ID:          "chrome",
		Description: "Chrome (auto-select latest)",
		Browser:     "Chrome",
		Version:     "auto",
		Deprecated:  false,
	},
	"firefox-120": {
		ID:          "firefox-120",
		Description: "Firefox 120 (current stable)",
		Browser:     "Firefox",
		Version:     "120",
		Deprecated:  false,
	},
	"firefox": {
		ID:          "firefox",
		Description: "Firefox (auto-select latest)",
		Browser:     "Firefox",
		Version:     "auto",
		Deprecated:  false,
	},
	"safari-16": {
		ID:          "safari-16",
		Description: "Safari 16 (macOS/iOS)",
		Browser:     "Safari",
		Version:     "16",
		Deprecated:  false,
	},
	"safari": {
		ID:          "safari",
		Description: "Safari (auto-detect version)",
		Browser:     "Safari",
		Version:     "auto",
		Deprecated:  false,
	},
	"edge-106": {
		ID:          "edge-106",
		Description: "Microsoft Edge 106",
		Browser:     "Edge",
		Version:     "106",
		Deprecated:  false,
	},
	"edge-85": {
		ID:          "edge-85",
		Description: "Microsoft Edge 85",
		Browser:     "Edge",
		Version:     "85",
		Deprecated:  true,
	},
	"edge": {
		ID:          "edge",
		Description: "Microsoft Edge (auto-detect version)",
		Browser:     "Edge",
		Version:     "auto",
		Deprecated:  false,
	},
	"auto": {
		ID:          "auto",
		Description: "Go default TLS (no fingerprint spoofing)",
		Browser:     "Go",
		Version:     "native",
		Deprecated:  false,
	},
	"randomized": {
		ID:          "randomized",
		Description: "Randomized fingerprint",
		Browser:     "Random",
		Version:     "random",
		Deprecated:  false,
	},
}

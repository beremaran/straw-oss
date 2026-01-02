// Package fingerprint provides a registry of browser fingerprint presets
// for spoofing TLS, HTTP/2, and header characteristics.
package fingerprint

import (
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
)

// HTTP2Settings represents HTTP/2 SETTINGS frame values used by browsers.
// These settings are fingerprinting vectors and differ between browsers.
type HTTP2Settings struct {
	HeaderTableSize      uint32
	EnablePush           bool
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	MaxHeaderListSize    uint32
}

// Preset represents a complete browser fingerprint profile.
// It includes TLS, HTTP/2, and header characteristics for accurate browser impersonation.
type Preset struct {
	// ID is the unique identifier for this preset (e.g., "chrome-133")
	ID string

	// TLSClientHello is the utls ClientHelloID for TLS fingerprinting
	TLSClientHello utls.ClientHelloID

	// HTTP2Settings contains the HTTP/2 SETTINGS frame values
	HTTP2Settings *HTTP2Settings

	// HeaderOrder specifies the exact header ordering for HTTP/1.1 requests
	HeaderOrder []string

	// PseudoHeaderOrder specifies HTTP/2 pseudo-header order (:method, :authority, etc.)
	PseudoHeaderOrder []string

	// UserAgent is the matching User-Agent string
	UserAgent string

	// AcceptLanguage is the locale-appropriate Accept-Language header
	AcceptLanguage string

	// SecCHUA is the Sec-CH-UA client hints header value
	SecCHUA string

	// SecCHUAMobile is the Sec-CH-UA-Mobile header value
	SecCHUAMobile string

	// SecCHUAPlatform is the Sec-CH-UA-Platform header value
	SecCHUAPlatform string

	// Deprecated indicates this preset should be retired
	Deprecated bool

	// LastUpdated is when this preset was last updated
	LastUpdated time.Time
}

// Registry provides thread-safe storage and retrieval of fingerprint presets.
type Registry struct {
	presets map[string]Preset
	mu      sync.RWMutex
}

// NewRegistry creates a new empty fingerprint registry.
func NewRegistry() *Registry {
	return &Registry{
		presets: make(map[string]Preset),
	}
}

// Get retrieves a fingerprint preset by its ID.
// Returns the preset and true if found, or zero value and false if not found.
func (r *Registry) Get(presetID string) (Preset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	preset, ok := r.presets[presetID]
	return preset, ok
}

// Register adds a new fingerprint preset to the registry.
// Returns an error if a preset with the same ID already exists.
func (r *Registry) Register(preset Preset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.presets[preset.ID]; exists {
		return &DuplicatePresetError{PresetID: preset.ID}
	}

	r.presets[preset.ID] = preset
	return nil
}

// MustRegister registers a preset and panics on error.
// This is intended for registering built-in presets at initialization.
func (r *Registry) MustRegister(preset Preset) {
	if err := r.Register(preset); err != nil {
		panic(err)
	}
}

// List returns a slice of all registered preset IDs.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.presets))
	for id := range r.presets {
		ids = append(ids, id)
	}
	return ids
}

// Count returns the number of registered presets.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.presets)
}

// DuplicatePresetError is returned when attempting to register a preset with a duplicate ID.
type DuplicatePresetError struct {
	PresetID string
}

func (e *DuplicatePresetError) Error() string {
	return "fingerprint preset already registered: " + e.PresetID
}

// defaultRegistry is the global registry instance with built-in presets.
var defaultRegistry *Registry

func init() {
	defaultRegistry = NewRegistry()
	registerBuiltinPresets(defaultRegistry)
}

// DefaultRegistry returns the global registry with built-in presets.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Get retrieves a preset from the default registry.
func Get(presetID string) (Preset, bool) {
	return defaultRegistry.Get(presetID)
}

// List returns all preset IDs from the default registry.
func List() []string {
	return defaultRegistry.List()
}

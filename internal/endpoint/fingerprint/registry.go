package fingerprint

import (
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
)

// HTTP2Settings configures HTTP/2 connection parameters.
type HTTP2Settings struct {
	HeaderTableSize      uint32
	EnablePush           bool
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	MaxHeaderListSize    uint32
}

// Preset holds a complete browser fingerprint configuration.
type Preset struct {
	ID                string
	TLSClientHello    utls.ClientHelloID
	HTTP2Settings     *HTTP2Settings
	HeaderOrder       []string
	PseudoHeaderOrder []string
	UserAgent         string
	AcceptLanguage    string
	SecCHUA           string
	SecCHUAMobile     string
	SecCHUAPlatform   string
	Deprecated        bool
	LastUpdated       time.Time
}

// Registry stores and retrieves browser fingerprint presets.
type Registry struct {
	presets map[string]Preset
	mu      sync.RWMutex
}

// NewRegistry creates an empty preset registry.
func NewRegistry() *Registry {
	return &Registry{
		presets: make(map[string]Preset),
	}
}

// Get retrieves a preset by its ID.
func (r *Registry) Get(presetID string) (Preset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	preset, ok := r.presets[presetID]

	return preset, ok
}

// Register adds a preset to the registry.
func (r *Registry) Register(preset Preset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.presets[preset.ID]; exists {
		return &DuplicatePresetError{PresetID: preset.ID}
	}

	r.presets[preset.ID] = preset

	return nil
}

// MustRegister registers a preset or panics on duplicate.
func (r *Registry) MustRegister(preset Preset) {
	err := r.Register(preset)
	if err != nil {
		panic(err)
	}
}

// List returns all registered preset IDs.
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

// DuplicatePresetError is returned when registering a preset with a duplicate ID.
type DuplicatePresetError struct {
	PresetID string
}

func (e *DuplicatePresetError) Error() string {
	return "fingerprint preset already registered: " + e.PresetID
}

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

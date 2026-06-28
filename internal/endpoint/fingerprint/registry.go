package fingerprint

import (
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
)

type HTTP2Settings struct {
	HeaderTableSize      uint32
	EnablePush           bool
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
	MaxFrameSize         uint32
	MaxHeaderListSize    uint32
}

type Preset struct {
	ID string

	TLSClientHello utls.ClientHelloID

	HTTP2Settings *HTTP2Settings

	HeaderOrder []string

	PseudoHeaderOrder []string

	UserAgent string

	AcceptLanguage string

	SecCHUA string

	SecCHUAMobile string

	SecCHUAPlatform string

	Deprecated bool

	LastUpdated time.Time
}

type Registry struct {
	presets map[string]Preset
	mu      sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		presets: make(map[string]Preset),
	}
}

func (r *Registry) Get(presetID string) (Preset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	preset, ok := r.presets[presetID]
	return preset, ok
}

func (r *Registry) Register(preset Preset) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.presets[preset.ID]; exists {
		return &DuplicatePresetError{PresetID: preset.ID}
	}

	r.presets[preset.ID] = preset
	return nil
}

func (r *Registry) MustRegister(preset Preset) {
	if err := r.Register(preset); err != nil {
		panic(err)
	}
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.presets))
	for id := range r.presets {
		ids = append(ids, id)
	}
	return ids
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.presets)
}

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

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func Get(presetID string) (Preset, bool) {
	return defaultRegistry.Get(presetID)
}

func List() []string {
	return defaultRegistry.List()
}

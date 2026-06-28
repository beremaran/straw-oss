package fingerprint

import (
	"sync"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if r.Count() != 0 {
		t.Errorf("expected empty registry, got %d presets", r.Count())
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	preset := Preset{
		ID:             "test-preset",
		TLSClientHello: utls.HelloGolang,
		UserAgent:      "Test/1.0",
		LastUpdated:    time.Now(),
	}

	err := r.Register(preset)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if r.Count() != 1 {
		t.Errorf("expected 1 preset, got %d", r.Count())
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()

	preset := Preset{
		ID:             "test-preset",
		TLSClientHello: utls.HelloGolang,
	}

	err := r.Register(preset)
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	err = r.Register(preset)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}

	var dupErr *DuplicatePresetError
	duplicatePresetError := &DuplicatePresetError{}
	if errors.As(err, &duplicatePresetError) {
		t.Errorf("expected DuplicatePresetError, got %T", err)
	} else {
		dupErr = func() *DuplicatePresetError {
			target := &DuplicatePresetError{}
			_ = errors.As(err, &target)
			return target
		}()
		if dupErr.PresetID != "test-preset" {
			t.Errorf("expected preset ID 'test-preset', got %q", dupErr.PresetID)
		}
	}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	preset := Preset{
		ID:             "chrome-test",
		TLSClientHello: utls.HelloChrome_133,
		UserAgent:      "Chrome Test",
		AcceptLanguage: "en-US",
	}

	_ = r.Register(preset)

	got, ok := r.Get("chrome-test")
	if !ok {
		t.Fatal("expected preset to be found")
	}
	if got.ID != "chrome-test" {
		t.Errorf("expected ID 'chrome-test', got %q", got.ID)
	}
	if got.UserAgent != "Chrome Test" {
		t.Errorf("expected UserAgent 'Chrome Test', got %q", got.UserAgent)
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	r := NewRegistry()

	_, ok := r.Get("unknown-preset")
	if ok {
		t.Error("expected preset to not be found")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	presets := []Preset{
		{ID: "preset-a", TLSClientHello: utls.HelloGolang},
		{ID: "preset-b", TLSClientHello: utls.HelloGolang},
		{ID: "preset-c", TLSClientHello: utls.HelloGolang},
	}

	for _, p := range presets {
		_ = r.Register(p)
	}

	list := r.List()
	if len(list) != 3 {
		t.Errorf("expected 3 presets in list, got %d", len(list))
	}

	found := make(map[string]bool)
	for _, id := range list {
		found[id] = true
	}

	for _, p := range presets {
		if !found[p.ID] {
			t.Errorf("preset %q not found in list", p.ID)
		}
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	for i := 0; i < 10; i++ {
		preset := Preset{
			ID:             "initial-" + string(rune('a'+i)),
			TLSClientHello: utls.HelloGolang,
		}
		_ = r.Register(preset)
	}

	var wg sync.WaitGroup
	const numGoroutines = 100
	const opsPerGoroutine = 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_ = r.List()
				_, _ = r.Get("initial-a")
				_ = r.Count()
			}
		}()
	}

	wg.Wait()
}

func TestDefaultRegistry_BuiltInPresets(t *testing.T) {
	r := DefaultRegistry()

	if r.Count() == 0 {
		t.Fatal("expected built-in presets to be registered")
	}

	expectedPresets := []string{
		"chrome-133",
		"chrome-131",
		"chrome-129",
		"firefox-133",
		"firefox-120",
		"safari-18",
		"safari-17",
		"edge-130",
		"edge-106",
	}

	for _, presetID := range expectedPresets {
		preset, ok := r.Get(presetID)
		if !ok {
			t.Errorf("expected built-in preset %q to exist", presetID)

			continue
		}

		if preset.ID != presetID {
			t.Errorf("preset %q: ID mismatch, got %q", presetID, preset.ID)
		}
		if preset.UserAgent == "" {
			t.Errorf("preset %q: UserAgent is empty", presetID)
		}
		if preset.AcceptLanguage == "" {
			t.Errorf("preset %q: AcceptLanguage is empty", presetID)
		}
		if preset.HTTP2Settings == nil {
			t.Errorf("preset %q: HTTP2Settings is nil", presetID)
		}
		if len(preset.HeaderOrder) == 0 {
			t.Errorf("preset %q: HeaderOrder is empty", presetID)
		}
		if len(preset.PseudoHeaderOrder) == 0 {
			t.Errorf("preset %q: PseudoHeaderOrder is empty", presetID)
		}
	}
}

func TestPreset_ChromeClientHints(t *testing.T) {
	preset, ok := Get("chrome-133")
	if !ok {
		t.Fatal("expected chrome-133 preset to exist")
	}

	if preset.SecCHUA == "" {
		t.Error("chrome preset should have SecCHUA")
	}
	if preset.SecCHUAMobile == "" {
		t.Error("chrome preset should have SecCHUAMobile")
	}
	if preset.SecCHUAPlatform == "" {
		t.Error("chrome preset should have SecCHUAPlatform")
	}
}

func TestPreset_FirefoxNoClientHints(t *testing.T) {
	preset, ok := Get("firefox-133")
	if !ok {
		t.Fatal("expected firefox-133 preset to exist")
	}

	if preset.SecCHUA != "" {
		t.Error("firefox preset should not have SecCHUA")
	}
	if preset.SecCHUAMobile != "" {
		t.Error("firefox preset should not have SecCHUAMobile")
	}
	if preset.SecCHUAPlatform != "" {
		t.Error("firefox preset should not have SecCHUAPlatform")
	}
}

func TestPreset_DeprecatedFlag(t *testing.T) {
	preset, ok := Get("chrome-129")
	if !ok {
		t.Fatal("expected chrome-129 preset to exist")
	}
	if !preset.Deprecated {
		t.Error("chrome-129 should be marked as deprecated")
	}

	preset, ok = Get("chrome-133")
	if !ok {
		t.Fatal("expected chrome-133 preset to exist")
	}
	if preset.Deprecated {
		t.Error("chrome-133 should not be marked as deprecated")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	preset, ok := Get("chrome-133")
	if !ok {
		t.Error("Get should find chrome-133")
	}
	if preset.ID != "chrome-133" {
		t.Error("Get returned wrong preset")
	}

	list := List()
	if len(list) == 0 {
		t.Error("List should return preset IDs")
	}
}

func TestMustRegister_Panic(t *testing.T) {
	r := NewRegistry()

	preset := Preset{
		ID:             "panic-test",
		TLSClientHello: utls.HelloGolang,
	}

	r.MustRegister(preset)

	defer func() {
		if recover() == nil {
			t.Error("MustRegister should panic on duplicate registration")
		}
	}()

	r.MustRegister(preset)
}

func TestHTTP2Settings(t *testing.T) {
	preset, ok := Get("chrome-133")
	if !ok {
		t.Fatal("expected chrome-133 preset")
	}

	settings := preset.HTTP2Settings
	if settings == nil {
		t.Fatal("HTTP2Settings should not be nil")
	}

	if settings.HeaderTableSize == 0 {
		t.Error("HeaderTableSize should be set")
	}
	if settings.InitialWindowSize == 0 {
		t.Error("InitialWindowSize should be set")
	}
	if settings.MaxConcurrentStreams == 0 {
		t.Error("MaxConcurrentStreams should be set")
	}
}

func TestDuplicatePresetError_Error(t *testing.T) {
	err := &DuplicatePresetError{PresetID: "test-id"}
	expected := "fingerprint preset already registered: test-id"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

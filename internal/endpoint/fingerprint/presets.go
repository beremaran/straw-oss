package fingerprint

import (
	"time"

	utls "github.com/refraction-networking/utls"
)

// registerBuiltinPresets registers all built-in browser fingerprint presets.
func registerBuiltinPresets(r *FingerprintRegistry) {
	// Chrome presets
	registerChromePresets(r)

	// Firefox presets
	registerFirefoxPresets(r)

	// Safari presets
	registerSafariPresets(r)

	// Edge presets
	registerEdgePresets(r)
}

// Common HTTP/2 pseudo-header orders
var (
	// chromePseudoHeaderOrder is the pseudo-header order used by Chrome
	chromePseudoHeaderOrder = []string{":method", ":authority", ":scheme", ":path"}

	// firefoxPseudoHeaderOrder is the pseudo-header order used by Firefox
	firefoxPseudoHeaderOrder = []string{":method", ":path", ":authority", ":scheme"}

	// safariPseudoHeaderOrder is the pseudo-header order used by Safari
	safariPseudoHeaderOrder = []string{":method", ":scheme", ":path", ":authority"}
)

// registerChromePresets registers Chrome browser fingerprint presets.
func registerChromePresets(r *FingerprintRegistry) {
	// Chrome 133 - Current stable
	r.MustRegister(FingerprintPreset{
		ID:             "chrome-133",
		TLSClientHello: utls.HelloChrome_133,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      65536,
			EnablePush:           false,
			MaxConcurrentStreams: 1000,
			InitialWindowSize:    6291456,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    262144,
		},
		HeaderOrder: []string{
			"Host",
			"Connection",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"Upgrade-Insecure-Requests",
			"User-Agent",
			"Accept",
			"Sec-Fetch-Site",
			"Sec-Fetch-Mode",
			"Sec-Fetch-User",
			"Sec-Fetch-Dest",
			"Accept-Encoding",
			"Accept-Language",
		},
		PseudoHeaderOrder: chromePseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           `"Chromium";v="133", "Not-A.Brand";v="24", "Google Chrome";v="133"`,
		SecCHUAMobile:     "?0",
		SecCHUAPlatform:   `"Windows"`,
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})

	// Chrome 131 - Previous version
	r.MustRegister(FingerprintPreset{
		ID:             "chrome-131",
		TLSClientHello: utls.HelloChrome_131,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      65536,
			EnablePush:           false,
			MaxConcurrentStreams: 1000,
			InitialWindowSize:    6291456,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    262144,
		},
		HeaderOrder: []string{
			"Host",
			"Connection",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"Upgrade-Insecure-Requests",
			"User-Agent",
			"Accept",
			"Sec-Fetch-Site",
			"Sec-Fetch-Mode",
			"Sec-Fetch-User",
			"Sec-Fetch-Dest",
			"Accept-Encoding",
			"Accept-Language",
		},
		PseudoHeaderOrder: chromePseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           `"Chromium";v="131", "Not-A.Brand";v="24", "Google Chrome";v="131"`,
		SecCHUAMobile:     "?0",
		SecCHUAPlatform:   `"Windows"`,
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC),
	})

	// Chrome 129 - Older version
	r.MustRegister(FingerprintPreset{
		ID:             "chrome-129",
		TLSClientHello: utls.HelloChrome_120,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      65536,
			EnablePush:           false,
			MaxConcurrentStreams: 1000,
			InitialWindowSize:    6291456,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    262144,
		},
		HeaderOrder: []string{
			"Host",
			"Connection",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"Upgrade-Insecure-Requests",
			"User-Agent",
			"Accept",
			"Sec-Fetch-Site",
			"Sec-Fetch-Mode",
			"Sec-Fetch-User",
			"Sec-Fetch-Dest",
			"Accept-Encoding",
			"Accept-Language",
		},
		PseudoHeaderOrder: chromePseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           `"Chromium";v="129", "Not-A.Brand";v="24", "Google Chrome";v="129"`,
		SecCHUAMobile:     "?0",
		SecCHUAPlatform:   `"Windows"`,
		Deprecated:        true,
		LastUpdated:       time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
	})
}

// registerFirefoxPresets registers Firefox browser fingerprint presets.
func registerFirefoxPresets(r *FingerprintRegistry) {
	// Firefox 133 - Current stable
	r.MustRegister(FingerprintPreset{
		ID:             "firefox-133",
		TLSClientHello: utls.HelloFirefox_120, // Using available Firefox preset
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      65536,
			EnablePush:           true,
			MaxConcurrentStreams: 100,
			InitialWindowSize:    131072,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    0, // Firefox doesn't send this
		},
		HeaderOrder: []string{
			"Host",
			"User-Agent",
			"Accept",
			"Accept-Language",
			"Accept-Encoding",
			"Connection",
			"Upgrade-Insecure-Requests",
			"Sec-Fetch-Dest",
			"Sec-Fetch-Mode",
			"Sec-Fetch-Site",
			"Sec-Fetch-User",
		},
		PseudoHeaderOrder: firefoxPseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
		AcceptLanguage:    "en-US,en;q=0.5",
		SecCHUA:           "", // Firefox doesn't send Sec-CH-UA
		SecCHUAMobile:     "",
		SecCHUAPlatform:   "",
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})

	// Firefox 120 - Previous version
	r.MustRegister(FingerprintPreset{
		ID:             "firefox-120",
		TLSClientHello: utls.HelloFirefox_120,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      65536,
			EnablePush:           true,
			MaxConcurrentStreams: 100,
			InitialWindowSize:    131072,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    0,
		},
		HeaderOrder: []string{
			"Host",
			"User-Agent",
			"Accept",
			"Accept-Language",
			"Accept-Encoding",
			"Connection",
			"Upgrade-Insecure-Requests",
			"Sec-Fetch-Dest",
			"Sec-Fetch-Mode",
			"Sec-Fetch-Site",
			"Sec-Fetch-User",
		},
		PseudoHeaderOrder: firefoxPseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",
		AcceptLanguage:    "en-US,en;q=0.5",
		SecCHUA:           "",
		SecCHUAMobile:     "",
		SecCHUAPlatform:   "",
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	})
}

// registerSafariPresets registers Safari browser fingerprint presets.
func registerSafariPresets(r *FingerprintRegistry) {
	// Safari 18 - macOS/iOS current stable
	r.MustRegister(FingerprintPreset{
		ID:             "safari-18",
		TLSClientHello: utls.HelloSafari_Auto,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      4096,
			EnablePush:           false,
			MaxConcurrentStreams: 100,
			InitialWindowSize:    2097152,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    0,
		},
		HeaderOrder: []string{
			"Host",
			"Accept",
			"Sec-Fetch-Site",
			"Accept-Language",
			"Sec-Fetch-Mode",
			"User-Agent",
			"Accept-Encoding",
			"Sec-Fetch-Dest",
			"Connection",
		},
		PseudoHeaderOrder: safariPseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           "", // Safari doesn't send Sec-CH-UA
		SecCHUAMobile:     "",
		SecCHUAPlatform:   "",
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})

	// Safari 17 - Previous version
	r.MustRegister(FingerprintPreset{
		ID:             "safari-17",
		TLSClientHello: utls.HelloSafari_16_0,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      4096,
			EnablePush:           false,
			MaxConcurrentStreams: 100,
			InitialWindowSize:    2097152,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    0,
		},
		HeaderOrder: []string{
			"Host",
			"Accept",
			"Sec-Fetch-Site",
			"Accept-Language",
			"Sec-Fetch-Mode",
			"User-Agent",
			"Accept-Encoding",
			"Sec-Fetch-Dest",
			"Connection",
		},
		PseudoHeaderOrder: safariPseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           "",
		SecCHUAMobile:     "",
		SecCHUAPlatform:   "",
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	})
}

// registerEdgePresets registers Microsoft Edge browser fingerprint presets.
func registerEdgePresets(r *FingerprintRegistry) {
	// Edge 130 - Current stable (based on Chromium)
	r.MustRegister(FingerprintPreset{
		ID:             "edge-130",
		TLSClientHello: utls.HelloEdge_Auto,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      65536,
			EnablePush:           false,
			MaxConcurrentStreams: 1000,
			InitialWindowSize:    6291456,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    262144,
		},
		HeaderOrder: []string{
			"Host",
			"Connection",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"Upgrade-Insecure-Requests",
			"User-Agent",
			"Accept",
			"Sec-Fetch-Site",
			"Sec-Fetch-Mode",
			"Sec-Fetch-User",
			"Sec-Fetch-Dest",
			"Accept-Encoding",
			"Accept-Language",
		},
		PseudoHeaderOrder: chromePseudoHeaderOrder, // Edge uses Chrome's order
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           `"Chromium";v="130", "Microsoft Edge";v="130", "Not-A.Brand";v="24"`,
		SecCHUAMobile:     "?0",
		SecCHUAPlatform:   `"Windows"`,
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})

	// Edge 106 - Earlier version
	r.MustRegister(FingerprintPreset{
		ID:             "edge-106",
		TLSClientHello: utls.HelloEdge_106,
		HTTP2Settings: &HTTP2Settings{
			HeaderTableSize:      65536,
			EnablePush:           false,
			MaxConcurrentStreams: 1000,
			InitialWindowSize:    6291456,
			MaxFrameSize:         16384,
			MaxHeaderListSize:    262144,
		},
		HeaderOrder: []string{
			"Host",
			"Connection",
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"Upgrade-Insecure-Requests",
			"User-Agent",
			"Accept",
			"Sec-Fetch-Site",
			"Sec-Fetch-Mode",
			"Sec-Fetch-User",
			"Sec-Fetch-Dest",
			"Accept-Encoding",
			"Accept-Language",
		},
		PseudoHeaderOrder: chromePseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/106.0.0.0 Safari/537.36 Edg/106.0.0.0",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           `"Chromium";v="106", "Microsoft Edge";v="106", "Not-A.Brand";v="24"`,
		SecCHUAMobile:     "?0",
		SecCHUAPlatform:   `"Windows"`,
		Deprecated:        true,
		LastUpdated:       time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	})
}

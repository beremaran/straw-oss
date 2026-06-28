package fingerprint

import (
	"time"

	utls "github.com/refraction-networking/utls"
)

func registerBuiltinPresets(r *Registry) {
	registerChromePresets(r)

	registerFirefoxPresets(r)

	registerSafariPresets(r)

	registerEdgePresets(r)
}

var (
	chromePseudoHeaderOrder = []string{":method", ":authority", ":scheme", ":path"}

	firefoxPseudoHeaderOrder = []string{":method", ":path", ":authority", ":scheme"}

	safariPseudoHeaderOrder = []string{":method", ":scheme", ":path", ":authority"}
)

//nolint:funlen
func registerChromePresets(r *Registry) {
	r.MustRegister(Preset{
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

	r.MustRegister(Preset{
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

	r.MustRegister(Preset{
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

//nolint:funlen
func registerFirefoxPresets(r *Registry) {
	r.MustRegister(Preset{
		ID:             "firefox-133",
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
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
		AcceptLanguage:    "en-US,en;q=0.5",
		SecCHUA:           "",
		SecCHUAMobile:     "",
		SecCHUAPlatform:   "",
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})

	r.MustRegister(Preset{
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

//nolint:funlen
func registerSafariPresets(r *Registry) {
	r.MustRegister(Preset{
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
		SecCHUA:           "",
		SecCHUAMobile:     "",
		SecCHUAPlatform:   "",
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})

	r.MustRegister(Preset{
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

//nolint:funlen
func registerEdgePresets(r *Registry) {
	r.MustRegister(Preset{
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
		PseudoHeaderOrder: chromePseudoHeaderOrder,
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           `"Chromium";v="130", "Microsoft Edge";v="130", "Not-A.Brand";v="24"`,
		SecCHUAMobile:     "?0",
		SecCHUAPlatform:   `"Windows"`,
		Deprecated:        false,
		LastUpdated:       time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	})

	r.MustRegister(Preset{
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

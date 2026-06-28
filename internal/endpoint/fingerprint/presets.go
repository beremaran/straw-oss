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

func registerChromePresets(r *Registry) {
	registerPresets(r,
		chromiumPreset(
			"chrome-133",
			utls.HelloChrome_133,
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
			`"Chromium";v="133", "Not-A.Brand";v="24", "Google Chrome";v="133"`,
			false,
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		),
		chromiumPreset(
			"chrome-131",
			utls.HelloChrome_131,
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			`"Chromium";v="131", "Not-A.Brand";v="24", "Google Chrome";v="131"`,
			false,
			time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC),
		),
		chromiumPreset(
			"chrome-129",
			utls.HelloChrome_120,
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36",
			`"Chromium";v="129", "Not-A.Brand";v="24", "Google Chrome";v="129"`,
			true,
			time.Date(2025, 9, 1, 0, 0, 0, 0, time.UTC),
		),
	)
}

func registerFirefoxPresets(r *Registry) {
	registerPresets(r,
		firefoxPreset(
			"firefox-133",
			utls.HelloFirefox_120,
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		),
		firefoxPreset(
			"firefox-120",
			utls.HelloFirefox_120,
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",
			time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		),
	)
}

func registerSafariPresets(r *Registry) {
	registerPresets(r,
		safariPreset(
			"safari-18",
			utls.HelloSafari_Auto,
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		),
		safariPreset(
			"safari-17",
			utls.HelloSafari_16_0,
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
			time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		),
	)
}

func registerEdgePresets(r *Registry) {
	registerPresets(r,
		chromiumPreset(
			"edge-130",
			utls.HelloEdge_Auto,
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0",
			`"Chromium";v="130", "Microsoft Edge";v="130", "Not-A.Brand";v="24"`,
			false,
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
		),
		chromiumPreset(
			"edge-106",
			utls.HelloEdge_106,
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/106.0.0.0 Safari/537.36 Edg/106.0.0.0",
			`"Chromium";v="106", "Microsoft Edge";v="106", "Not-A.Brand";v="24"`,
			true,
			time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		),
	)
}

func registerPresets(r *Registry, presets ...Preset) {
	for _, preset := range presets {
		r.MustRegister(preset)
	}
}

func chromiumPreset(
	id string,
	hello utls.ClientHelloID,
	userAgent string,
	secCHUA string,
	deprecated bool,
	lastUpdated time.Time,
) Preset {
	return Preset{
		ID:                id,
		TLSClientHello:    hello,
		HTTP2Settings:     chromiumHTTP2Settings(),
		HeaderOrder:       chromiumHeaderOrder(),
		PseudoHeaderOrder: chromePseudoHeaderOrder,
		UserAgent:         userAgent,
		AcceptLanguage:    "en-US,en;q=0.9",
		SecCHUA:           secCHUA,
		SecCHUAMobile:     "?0",
		SecCHUAPlatform:   `"Windows"`,
		Deprecated:        deprecated,
		LastUpdated:       lastUpdated,
	}
}

func chromiumHTTP2Settings() *HTTP2Settings {
	return &HTTP2Settings{
		HeaderTableSize:      65536,
		EnablePush:           false,
		MaxConcurrentStreams: 1000,
		InitialWindowSize:    6291456,
		MaxFrameSize:         16384,
		MaxHeaderListSize:    262144,
	}
}

func chromiumHeaderOrder() []string {
	return []string{
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
	}
}

func firefoxPreset(id string, hello utls.ClientHelloID, userAgent string, lastUpdated time.Time) Preset {
	return Preset{
		ID:                id,
		TLSClientHello:    hello,
		HTTP2Settings:     firefoxHTTP2Settings(),
		HeaderOrder:       firefoxHeaderOrder(),
		PseudoHeaderOrder: firefoxPseudoHeaderOrder,
		UserAgent:         userAgent,
		AcceptLanguage:    "en-US,en;q=0.5",
		LastUpdated:       lastUpdated,
	}
}

func firefoxHTTP2Settings() *HTTP2Settings {
	return &HTTP2Settings{
		HeaderTableSize:      65536,
		EnablePush:           true,
		MaxConcurrentStreams: 100,
		InitialWindowSize:    131072,
		MaxFrameSize:         16384,
		MaxHeaderListSize:    0,
	}
}

func firefoxHeaderOrder() []string {
	return []string{
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
	}
}

func safariPreset(id string, hello utls.ClientHelloID, userAgent string, lastUpdated time.Time) Preset {
	return Preset{
		ID:                id,
		TLSClientHello:    hello,
		HTTP2Settings:     safariHTTP2Settings(),
		HeaderOrder:       safariHeaderOrder(),
		PseudoHeaderOrder: safariPseudoHeaderOrder,
		UserAgent:         userAgent,
		AcceptLanguage:    "en-US,en;q=0.9",
		LastUpdated:       lastUpdated,
	}
}

func safariHTTP2Settings() *HTTP2Settings {
	return &HTTP2Settings{
		HeaderTableSize:      4096,
		EnablePush:           false,
		MaxConcurrentStreams: 100,
		InitialWindowSize:    2097152,
		MaxFrameSize:         16384,
		MaxHeaderListSize:    0,
	}
}

func safariHeaderOrder() []string {
	return []string{
		"Host",
		"Accept",
		"Sec-Fetch-Site",
		"Accept-Language",
		"Sec-Fetch-Mode",
		"User-Agent",
		"Accept-Encoding",
		"Sec-Fetch-Dest",
		"Connection",
	}
}

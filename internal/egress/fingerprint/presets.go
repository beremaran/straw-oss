// Package fingerprint provides HTTP fingerprint presets for simulating
// different browsers' TLS and HTTP/2 characteristics.
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

// DefaultPresetID is the ID of the default browser fingerprint preset.
const DefaultPresetID = "chrome-133"

const authorityPseudoHeader = ":authority"

const methodPseudoHeader = ":method"

const pathPseudoHeader = ":path"

const schemePseudoHeader = ":scheme"

const hostHeader = "Host"

const connectionHeader = "Connection"

const userAgentHeader = "User-Agent"

const acceptHeader = "Accept"

const secFetchSiteHeader = "Sec-Fetch-Site"

const secFetchModeHeader = "Sec-Fetch-Mode"

const secFetchDestHeader = "Sec-Fetch-Dest"

const acceptEncodingHeader = "Accept-Encoding"

const acceptLanguageHeader = "Accept-Language"

// HTTP/2 settings magic numbers.
const (
	chromiumHeaderTableSize      = 65536
	chromiumMaxConcurrentStreams = 1000
	chromiumInitialWindowSize    = 6291456
	chromiumMaxFrameSize         = 16384
	chromiumMaxHeaderListSize    = 262144

	firefoxHeaderTableSize      = 65536
	firefoxMaxConcurrentStreams = 100
	firefoxInitialWindowSize    = 131072
	firefoxMaxFrameSize         = 16384

	safariHeaderTableSize      = 4096
	safariMaxConcurrentStreams = 100
	safariInitialWindowSize    = 2097152
	safariMaxFrameSize         = 16384
)

var (
	chromePseudoHeaderOrder = []string{methodPseudoHeader, authorityPseudoHeader, schemePseudoHeader, pathPseudoHeader}

	firefoxPseudoHeaderOrder = []string{methodPseudoHeader, pathPseudoHeader, authorityPseudoHeader, schemePseudoHeader}

	safariPseudoHeaderOrder = []string{methodPseudoHeader, schemePseudoHeader, pathPseudoHeader, authorityPseudoHeader}
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
		HeaderTableSize:      chromiumHeaderTableSize,
		EnablePush:           false,
		MaxConcurrentStreams: chromiumMaxConcurrentStreams,
		InitialWindowSize:    chromiumInitialWindowSize,
		MaxFrameSize:         chromiumMaxFrameSize,
		MaxHeaderListSize:    chromiumMaxHeaderListSize,
	}
}

func chromiumHeaderOrder() []string {
	return []string{
		hostHeader,
		connectionHeader,
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"Upgrade-Insecure-Requests",
		userAgentHeader,
		acceptHeader,
		secFetchSiteHeader,
		secFetchModeHeader,
		"Sec-Fetch-User",
		secFetchDestHeader,
		acceptEncodingHeader,
		acceptLanguageHeader,
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
		HeaderTableSize:      firefoxHeaderTableSize,
		EnablePush:           true,
		MaxConcurrentStreams: firefoxMaxConcurrentStreams,
		InitialWindowSize:    firefoxInitialWindowSize,
		MaxFrameSize:         firefoxMaxFrameSize,
		MaxHeaderListSize:    0,
	}
}

func firefoxHeaderOrder() []string {
	return []string{
		hostHeader,
		userAgentHeader,
		acceptHeader,
		acceptLanguageHeader,
		acceptEncodingHeader,
		connectionHeader,
		"Upgrade-Insecure-Requests",
		secFetchDestHeader,
		secFetchModeHeader,
		secFetchSiteHeader,
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
		HeaderTableSize:      safariHeaderTableSize,
		EnablePush:           false,
		MaxConcurrentStreams: safariMaxConcurrentStreams,
		InitialWindowSize:    safariInitialWindowSize,
		MaxFrameSize:         safariMaxFrameSize,
		MaxHeaderListSize:    0,
	}
}

func safariHeaderOrder() []string {
	return []string{
		hostHeader,
		acceptHeader,
		secFetchSiteHeader,
		acceptLanguageHeader,
		secFetchModeHeader,
		userAgentHeader,
		acceptEncodingHeader,
		secFetchDestHeader,
		connectionHeader,
	}
}

package http

import (
	"bytes"
	"context"
	"net/url"
	"sort"
	"strings"

	fhttp "github.com/useflyent/fhttp"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// BuildRequest converts a protocol.Request to an fhttp.Request with proper header ordering.
func BuildRequest(ctx context.Context, req *protocol.Request, preset fingerprint.FingerprintPreset) (*fhttp.Request, error) {
	// Parse URL
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		return nil, &ClientError{
			Code:    "INVALID_URL",
			Message: "failed to parse URL: " + err.Error(),
		}
	}

	// Create request body
	var bodyReader *bytes.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	// Create fhttp request
	var fhttpReq *fhttp.Request
	if bodyReader != nil {
		fhttpReq, err = fhttp.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	} else {
		fhttpReq, err = fhttp.NewRequestWithContext(ctx, req.Method, req.URL, nil)
	}
	if err != nil {
		return nil, &ClientError{
			Code:    "REQUEST_BUILD_FAILED",
			Message: "failed to create request: " + err.Error(),
		}
	}

	// Set Host header from URL
	fhttpReq.Host = parsedURL.Host

	// Apply headers from protocol request
	applyHeaders(fhttpReq, req.Headers)

	// Apply fingerprint-specific header order
	applyHeaderOrder(fhttpReq, preset.HeaderOrder)

	// Set pseudo-header order for HTTP/2
	if len(preset.PseudoHeaderOrder) > 0 {
		fhttpReq.Header[fhttp.PHeaderOrderKey] = preset.PseudoHeaderOrder
	}

	// Apply fingerprint-specific headers if not already set
	applyFingerprintHeaders(fhttpReq, preset)

	return fhttpReq, nil
}

// applyHeaders copies headers from protocol.HeaderMap to fhttp.Header.
func applyHeaders(req *fhttp.Request, headers protocol.HeaderMap) {
	for _, h := range headers {
		// Skip pseudo-headers and host (handled separately)
		if strings.HasPrefix(h.Key, ":") || strings.EqualFold(h.Key, "host") {
			continue
		}
		req.Header.Set(h.Key, h.Value)
	}
}

// applyHeaderOrder sets the header order key for fhttp.
func applyHeaderOrder(req *fhttp.Request, order []string) {
	if len(order) == 0 {
		return
	}

	// Get current headers and sort them according to the order
	orderedHeaders := make([]string, 0, len(req.Header))
	orderMap := make(map[string]int)
	for i, h := range order {
		orderMap[strings.ToLower(h)] = i
	}

	// Collect all header keys
	for key := range req.Header {
		if key == fhttp.PHeaderOrderKey || key == fhttp.HeaderOrderKey {
			continue
		}
		orderedHeaders = append(orderedHeaders, key)
	}

	// Sort headers according to fingerprint order
	sort.SliceStable(orderedHeaders, func(i, j int) bool {
		iOrder, iExists := orderMap[strings.ToLower(orderedHeaders[i])]
		jOrder, jExists := orderMap[strings.ToLower(orderedHeaders[j])]

		if iExists && jExists {
			return iOrder < jOrder
		}
		if iExists {
			return true
		}
		if jExists {
			return false
		}
		// Both not in order, maintain original order
		return i < j
	})

	// Set header order
	req.Header[fhttp.HeaderOrderKey] = orderedHeaders
}

// applyFingerprintHeaders applies browser-specific headers from the fingerprint preset.
func applyFingerprintHeaders(req *fhttp.Request, preset fingerprint.FingerprintPreset) {
	// Set User-Agent if not already set and preset has one
	if req.Header.Get("User-Agent") == "" && preset.UserAgent != "" {
		req.Header.Set("User-Agent", preset.UserAgent)
	}

	// Set Accept-Language if not already set
	if req.Header.Get("Accept-Language") == "" && preset.AcceptLanguage != "" {
		req.Header.Set("Accept-Language", preset.AcceptLanguage)
	}

	// Set Sec-CH-UA headers if not already set
	if req.Header.Get("Sec-CH-UA") == "" && preset.SecCHUA != "" {
		req.Header.Set("Sec-CH-UA", preset.SecCHUA)
	}
	if req.Header.Get("Sec-CH-UA-Mobile") == "" && preset.SecCHUAMobile != "" {
		req.Header.Set("Sec-CH-UA-Mobile", preset.SecCHUAMobile)
	}
	if req.Header.Get("Sec-CH-UA-Platform") == "" && preset.SecCHUAPlatform != "" {
		req.Header.Set("Sec-CH-UA-Platform", preset.SecCHUAPlatform)
	}

	// Set common browser headers if not already set
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	}
	if req.Header.Get("Accept-Encoding") == "" {
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	}
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "keep-alive")
	}
}

// HeadersToProtocol converts fhttp.Header to protocol.HeaderMap preserving order.
func HeadersToProtocol(headers fhttp.Header) protocol.HeaderMap {
	result := make(protocol.HeaderMap, 0, len(headers))
	for key, values := range headers {
		// Skip internal fhttp headers
		if key == fhttp.PHeaderOrderKey || key == fhttp.HeaderOrderKey {
			continue
		}
		for _, value := range values {
			result = append(result, protocol.Header{Key: key, Value: value})
		}
	}
	return result
}

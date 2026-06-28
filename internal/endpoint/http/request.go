package http

import (
	"bytes"
	"context"
	"net/url"
	"sort"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
	"github.com/beremaran/straw/pkg/protocol"
)

func BuildRequest(ctx context.Context, req *protocol.Request, preset fingerprint.Preset) (*fhttp.Request, error) {

	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		return nil, &ClientError{
			Code:    "INVALID_URL",
			Message: "failed to parse URL: " + err.Error(),
		}
	}

	var bodyReader *bytes.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

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

	fhttpReq.Host = parsedURL.Host

	applyHeaders(fhttpReq, req.Headers)

	applyHeaderOrder(fhttpReq, preset.HeaderOrder)

	if len(preset.PseudoHeaderOrder) > 0 {
		fhttpReq.Header[fhttp.PHeaderOrderKey] = preset.PseudoHeaderOrder
	}

	applyFingerprintHeaders(fhttpReq, preset)

	return fhttpReq, nil
}

func applyHeaders(req *fhttp.Request, headers protocol.HeaderMap) {
	for _, h := range headers {

		if strings.HasPrefix(h.Key, ":") || strings.EqualFold(h.Key, "host") {
			continue
		}
		req.Header.Set(h.Key, h.Value)
	}
}

func applyHeaderOrder(req *fhttp.Request, order []string) {
	if len(order) == 0 {
		return
	}

	orderedHeaders := make([]string, 0, len(req.Header))
	orderMap := make(map[string]int)
	for i, h := range order {
		orderMap[strings.ToLower(h)] = i
	}

	for key := range req.Header {
		if key == fhttp.PHeaderOrderKey || key == fhttp.HeaderOrderKey {
			continue
		}
		orderedHeaders = append(orderedHeaders, key)
	}

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

		return i < j
	})

	req.Header[fhttp.HeaderOrderKey] = orderedHeaders
}

func applyFingerprintHeaders(req *fhttp.Request, preset fingerprint.Preset) {

	if req.Header.Get("User-Agent") == "" && preset.UserAgent != "" {
		req.Header.Set("User-Agent", preset.UserAgent)
	}

	if req.Header.Get("Accept-Language") == "" && preset.AcceptLanguage != "" {
		req.Header.Set("Accept-Language", preset.AcceptLanguage)
	}

	if req.Header.Get("Sec-CH-UA") == "" && preset.SecCHUA != "" {
		req.Header.Set("Sec-CH-UA", preset.SecCHUA)
	}
	if req.Header.Get("Sec-CH-UA-Mobile") == "" && preset.SecCHUAMobile != "" {
		req.Header.Set("Sec-CH-UA-Mobile", preset.SecCHUAMobile)
	}
	if req.Header.Get("Sec-CH-UA-Platform") == "" && preset.SecCHUAPlatform != "" {
		req.Header.Set("Sec-CH-UA-Platform", preset.SecCHUAPlatform)
	}

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

func HeadersToProtocol(headers fhttp.Header) protocol.HeaderMap {
	result := make(protocol.HeaderMap, 0, len(headers))
	for key, values := range headers {

		if key == fhttp.PHeaderOrderKey || key == fhttp.HeaderOrderKey {
			continue
		}
		for _, value := range values {
			result = append(result, protocol.Header{Key: key, Value: value})
		}
	}
	return result
}

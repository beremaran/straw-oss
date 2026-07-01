package http

import (
	"bytes"
	"context"
	"net/url"
	"sort"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/protocol"
)

const (
	chromeUserAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
	chromeAcceptLanguage  = "en-US,en;q=0.9"
	chromeSecCHUA         = `"Chromium";v="133", "Not-A.Brand";v="24", "Google Chrome";v="133"`
	chromeSecCHUAMobile   = "?0"
	chromeSecCHUAPlatform = `"Windows"`
	userAgentHeader       = "User-Agent"
	acceptHeader          = "Accept"
	acceptAll             = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"
)

var (
	chromeHeaderOrder = []string{
		"Host",
		"Connection",
		"sec-ch-ua",
		"sec-ch-ua-mobile",
		"sec-ch-ua-platform",
		"Upgrade-Insecure-Requests",
		userAgentHeader,
		acceptHeader,
		"Sec-Fetch-Site",
		"Sec-Fetch-Mode",
		"Sec-Fetch-User",
		"Sec-Fetch-Dest",
		"Accept-Encoding",
		"Accept-Language",
	}
	chromePseudoHeaderOrder = []string{":method", ":authority", ":scheme", ":path"}
)

// BuildRequest creates an fhttp.Request from a protocol request with Chrome headers applied.
func BuildRequest(ctx context.Context, req *protocol.Request) (*fhttp.Request, error) {
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

	applyHeaderOrder(fhttpReq, chromeHeaderOrder)

	fhttpReq.Header[fhttp.PHeaderOrderKey] = chromePseudoHeaderOrder

	applyDefaultChromeHeaders(fhttpReq)

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

func applyDefaultChromeHeaders(req *fhttp.Request) {
	for _, h := range defaultChromeHeaders() {
		setHeaderDefault(req, h.key, h.value)
	}
}

type defaultHeader struct {
	key   string
	value string
}

func defaultChromeHeaders() []defaultHeader {
	return []defaultHeader{
		{key: userAgentHeader, value: chromeUserAgent},
		{key: "Accept-Language", value: chromeAcceptLanguage},
		{key: "Sec-CH-UA", value: chromeSecCHUA},
		{key: "Sec-CH-UA-Mobile", value: chromeSecCHUAMobile},
		{key: "Sec-CH-UA-Platform", value: chromeSecCHUAPlatform},
		{key: acceptHeader, value: acceptAll},
		{key: "Accept-Encoding", value: "gzip, deflate, br"},
		{key: "Connection", value: "keep-alive"},
	}
}

func setHeaderDefault(req *fhttp.Request, key, value string) {
	if value != "" && req.Header.Get(key) == "" {
		req.Header.Set(key, value)
	}
}

// HeadersToProtocol converts fhttp headers to protocol headers.
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

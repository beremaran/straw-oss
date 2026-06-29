package http

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/pkg/protocol"
)

// BuildResponse creates a protocol response from an fhttp response.
func BuildResponse(
	requestID string,
	resp *fhttp.Response,
	timing protocol.TimingInfo,
	maxBodySize int64,
	endpointID string,
	sessionID string,
) (*protocol.Response, error) {
	body, err := readResponseBody(resp, maxBodySize)
	if err != nil {
		return &protocol.Response{
			RequestID:  requestID,
			StatusCode: resp.StatusCode,
			Headers:    HeadersToProtocol(resp.Header),
			EndpointID: endpointID,
			SessionID:  sessionID,
			Timing:     &timing,
			Error: &protocol.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   "failed to read response body: " + err.Error(),
				Retryable: false,
			},
		}, err
	}

	return &protocol.Response{
		RequestID:  requestID,
		StatusCode: resp.StatusCode,
		Headers:    HeadersToProtocol(resp.Header),
		Body:       body,
		EndpointID: endpointID,
		SessionID:  sessionID,
		Timing:     &timing,
	}, nil
}

// ResponseOptions configures response building behavior.
type ResponseOptions struct {
	MaxBodySize    int64
	StreamResponse bool
}

// BuildResponseWithOptions creates a protocol response with custom options.
func BuildResponseWithOptions(
	requestID string,
	resp *fhttp.Response,
	timing protocol.TimingInfo,
	opts ResponseOptions,
	endpointID string,
	sessionID string,
) (*protocol.Response, error) {
	if opts.StreamResponse {
		return buildStreamingResponse(requestID, resp, timing, endpointID, sessionID)
	}

	maxSize := opts.MaxBodySize
	if maxSize <= 0 {
		maxSize = DefaultMaxBodySize
	}

	return BuildResponse(requestID, resp, timing, maxSize, endpointID, sessionID)
}

func buildStreamingResponse(
	requestID string,
	resp *fhttp.Response,
	timing protocol.TimingInfo,
	endpointID string,
	sessionID string,
) (*protocol.Response, error) {
	return &protocol.Response{
		RequestID:   requestID,
		StatusCode:  resp.StatusCode,
		Headers:     HeadersToProtocol(resp.Header),
		IsStreaming: true,
		EndpointID:  endpointID,
		SessionID:   sessionID,
		Timing:      &timing,
	}, nil
}

func readResponseBody(resp *fhttp.Response, maxSize int64) ([]byte, error) {
	rawBody, err := readRawResponseBody(resp, maxSize)
	if err != nil {
		return nil, err
	}

	return decodeResponseBody(rawBody, resp.Header.Get("Content-Encoding")), nil
}

func readRawResponseBody(resp *fhttp.Response, maxSize int64) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	limitReader := io.LimitReader(resp.Body, maxSize+1)

	rawBody, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, &ClientError{
			Code:    "BODY_READ_FAILED",
			Message: "failed to read response body: " + err.Error(),
		}
	}

	if int64(len(rawBody)) > maxSize {
		rawBody = rawBody[:maxSize]
	}

	return rawBody, nil
}

func decodeResponseBody(rawBody []byte, contentEncoding string) []byte {
	switch strings.ToLower(contentEncoding) {
	case "gzip":
		return decodeGzipBody(rawBody)
	case "br":
		return decodeBrotliBody(rawBody)
	default:
		return rawBody
	}
}

func decodeGzipBody(rawBody []byte) []byte {
	if len(rawBody) < 2 || rawBody[0] != 0x1f || rawBody[1] != 0x8b {
		return rawBody
	}

	gzReader, err := gzip.NewReader(bytes.NewReader(rawBody))
	if err != nil {
		return rawBody
	}
	defer func() { _ = gzReader.Close() }()

	decompressed, err := io.ReadAll(gzReader)
	if err != nil {
		return rawBody
	}

	return decompressed
}

func decodeBrotliBody(rawBody []byte) []byte {
	brReader := brotli.NewReader(bytes.NewReader(rawBody))

	decompressed, err := io.ReadAll(brReader)
	if err != nil {
		return rawBody
	}

	return decompressed
}

// IsSuccessStatus reports whether the status code indicates success.
func IsSuccessStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// IsRedirectStatus reports whether the status code indicates a redirect.
func IsRedirectStatus(statusCode int) bool {
	return statusCode >= 300 && statusCode < 400
}

// IsClientErrorStatus reports whether the status code indicates a client error.
func IsClientErrorStatus(statusCode int) bool {
	return statusCode >= 400 && statusCode < 500
}

// IsServerErrorStatus reports whether the status code indicates a server error.
func IsServerErrorStatus(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}

// IsRetryableStatus reports whether the status code indicates a retryable error.
func IsRetryableStatus(statusCode int) bool {
	switch statusCode {
	case StatusCodeRetryable429, StatusCodeRetryable500, StatusCodeRetryable502, StatusCodeRetryable503, StatusCodeRetryable504:
		return true
	default:
		return false
	}
}

// ShouldEscalatePool reports whether the status code indicates pool escalation is needed.
func ShouldEscalatePool(statusCode int) bool {
	switch statusCode {
	case StatusCodeEscalate403, StatusCodeEscalate407:
		return true
	default:
		return false
	}
}

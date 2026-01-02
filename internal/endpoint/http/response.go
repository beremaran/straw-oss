package http

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	fhttp "github.com/useflyent/fhttp"

	"github.com/kwilabs/straw-proxy-server/pkg/protocol"
)

// BuildResponse converts an fhttp.Response to a protocol.Response.
func BuildResponse(
	requestID string,
	resp *fhttp.Response,
	timing protocol.TimingInfo,
	maxBodySize int64,
	endpointID string,
	sessionID string,
) (*protocol.Response, error) {
	// Read and decompress body
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
		}, nil
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

// readResponseBody reads and decompresses the response body.
// It handles the case where the HTTP/2 transport may have already decompressed
// the body transparently (while leaving the Content-Encoding header intact).
func readResponseBody(resp *fhttp.Response, maxSize int64) ([]byte, error) {
	if resp.Body == nil {
		return nil, nil
	}

	// First, read the raw body
	limitReader := io.LimitReader(resp.Body, maxSize+1)
	rawBody, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, &ClientError{
			Code:    "BODY_READ_FAILED",
			Message: "failed to read response body: " + err.Error(),
		}
	}

	// Check if body exceeded max size
	if int64(len(rawBody)) > maxSize {
		rawBody = rawBody[:maxSize] // Truncate but don't error
	}

	// Check if decompression is needed based on Content-Encoding header
	contentEncoding := strings.ToLower(resp.Header.Get("Content-Encoding"))

	switch contentEncoding {
	case "gzip":
		// Check if body starts with gzip magic bytes (0x1f, 0x8b)
		if len(rawBody) >= 2 && rawBody[0] == 0x1f && rawBody[1] == 0x8b {
			gzReader, err := gzip.NewReader(bytes.NewReader(rawBody))
			if err != nil {
				// If gzip reader creation fails, body might already be decompressed
				return rawBody, nil
			}
			defer gzReader.Close()
			decompressed, err := io.ReadAll(gzReader)
			if err != nil {
				// Decompression failed, return raw body (might already be decompressed)
				return rawBody, nil
			}
			return decompressed, nil
		}
		// No gzip magic bytes, body is likely already decompressed
		return rawBody, nil

	case "br":
		// Brotli doesn't have a simple magic number, but we can try to decompress
		// and fall back to raw body if it fails (indicating already decompressed)
		brReader := brotli.NewReader(bytes.NewReader(rawBody))
		decompressed, err := io.ReadAll(brReader)
		if err != nil {
			// Decompression failed, body is likely already decompressed by transport
			// This commonly happens with HTTP/2 where the transport handles decompression
			// but doesn't remove the Content-Encoding header
			return rawBody, nil
		}
		return decompressed, nil

	case "identity", "":
		// No compression, use body directly
		return rawBody, nil

	default:
		// Unknown encoding, return as-is
		return rawBody, nil
	}
}

// IsSuccessStatus returns true if the status code indicates success (2xx).
func IsSuccessStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// IsRedirectStatus returns true if the status code indicates a redirect (3xx).
func IsRedirectStatus(statusCode int) bool {
	return statusCode >= 300 && statusCode < 400
}

// IsClientErrorStatus returns true if the status code indicates a client error (4xx).
func IsClientErrorStatus(statusCode int) bool {
	return statusCode >= 400 && statusCode < 500
}

// IsServerErrorStatus returns true if the status code indicates a server error (5xx).
func IsServerErrorStatus(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}

// IsRetryableStatus returns true if the status code indicates a retryable error.
func IsRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 429, // Too Many Requests
		500, // Internal Server Error
		502, // Bad Gateway
		503, // Service Unavailable
		504: // Gateway Timeout
		return true
	default:
		return false
	}
}

// ShouldEscalatePool returns true if the status code indicates we should try a different endpoint pool.
func ShouldEscalatePool(statusCode int) bool {
	switch statusCode {
	case 403, // Forbidden (often captcha/block)
		407: // Proxy Authentication Required
		return true
	default:
		return false
	}
}

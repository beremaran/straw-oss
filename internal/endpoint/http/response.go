package http

import (
	"bytes"
	"compress/gzip"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/protocol"
)

// BuildResponse creates a protocol response from an fhttp response.
func BuildResponse(
	requestID string,
	resp *fhttp.Response,
	timing protocol.TimingInfo,
	maxBodySize int64,
	endpointID string,
) (*protocol.Response, error) {
	body, err := readResponseBody(resp, maxBodySize)
	if err != nil {
		return &protocol.Response{
			RequestID:  requestID,
			StatusCode: resp.StatusCode,
			Headers:    HeadersToProtocol(resp.Header),
			EndpointID: endpointID,
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
		Timing:     &timing,
	}, nil
}

// ResponseOptions configures response building behavior.
type ResponseOptions struct {
	MaxBodySize int64
}

// BuildResponseWithOptions creates a protocol response with custom options.
func BuildResponseWithOptions(
	requestID string,
	resp *fhttp.Response,
	timing protocol.TimingInfo,
	opts ResponseOptions,
	endpointID string,
) (*protocol.Response, error) {
	maxSize := opts.MaxBodySize
	if maxSize <= 0 {
		maxSize = DefaultMaxBodySize
	}

	return BuildResponse(requestID, resp, timing, maxSize, endpointID)
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

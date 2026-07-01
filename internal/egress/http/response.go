package http

import (
	"io"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/protocol"
)

// BuildResponse creates a protocol response from an fhttp response.
func BuildResponse(
	requestID string,
	resp *fhttp.Response,
	timing protocol.TimingInfo,
	maxBodySize int64,
	egressID string,
) (*protocol.Response, error) {
	body, err := readResponseBody(resp, maxBodySize)
	if err != nil {
		return &protocol.Response{
			RequestID:  requestID,
			StatusCode: resp.StatusCode,
			Headers:    HeadersToProtocol(resp.Header),
			EgressID:   egressID,
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
		EgressID:   egressID,
		Timing:     &timing,
	}, nil
}


func readResponseBody(resp *fhttp.Response, maxSize int64) ([]byte, error) {
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

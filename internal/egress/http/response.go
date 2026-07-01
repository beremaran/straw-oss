package http

import (
	"io"
	"math"

	fhttp "github.com/bogdanfinn/fhttp"

	"github.com/beremaran/straw/internal/protocol"
	"github.com/beremaran/straw/internal/protocol/wirepb"
)

// BuildResponse creates a protocol response from an fhttp response.
func BuildResponse(
	requestID string,
	resp *fhttp.Response,
	timing *wirepb.TimingInfo,
	maxBodySize int64,
	egressID string,
) (*wirepb.Response, error) {
	body, err := readResponseBody(resp, maxBodySize)
	if err != nil {
		return &wirepb.Response{
			RequestId:  requestID,
			StatusCode: statusCodeToWire(resp.StatusCode),
			Headers:    HeadersToProtocol(resp.Header),
			EgressId:   egressID,
			Timing:     timing,
			Error: &wirepb.ErrorInfo{
				Code:      protocol.ErrCodeUpstreamError,
				Message:   "failed to read response body: " + err.Error(),
				Retryable: false,
			},
		}, err
	}

	return &wirepb.Response{
		RequestId:  requestID,
		StatusCode: statusCodeToWire(resp.StatusCode),
		Headers:    HeadersToProtocol(resp.Header),
		Body:       body,
		EgressId:   egressID,
		Timing:     timing,
	}, nil
}

func statusCodeToWire(status int) int32 {
	if status > math.MaxInt32 {
		return math.MaxInt32
	}

	if status < math.MinInt32 {
		return math.MinInt32
	}

	return int32(status)
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

// Package dto defines control HTTP request and response DTOs.
package dto

import (
	"fmt"
	"time"

	"github.com/beremaran/straw/internal/protocol"
)

// ToProtocolRequest converts a ControlRequest to a protocol.Request.
func (r *ControlRequest) ToProtocolRequest() (*protocol.Request, error) {
	var timeout time.Duration

	if r.Timeout != "" {
		var err error

		timeout, err = time.ParseDuration(r.Timeout)
		if err != nil {
			return nil, fmt.Errorf("parse timeout: %w", err)
		}
	}

	headers := make(protocol.HeaderMap, 0, len(r.Headers))
	for k, v := range r.Headers {
		headers = append(headers, protocol.Header{Key: k, Value: v})
	}

	return &protocol.Request{
		ID:              r.ID,
		Method:          r.Method,
		URL:             r.URL,
		Headers:         headers,
		Body:            r.Body,
		Timeout:         timeout,
		MaxResponseSize: r.MaxResponseSize,
	}, nil
}

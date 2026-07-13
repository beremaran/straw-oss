// Package main demonstrates the tagged Straw Go client.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	straw "github.com/beremaran/straw-sdk-go"
)

const (
	clientTimeout  = 20 * time.Second
	requestTimeout = 15_000
)

var errAPIRequest = errors.New("straw rejected request")

func main() {
	err := run()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

func run() error {
	baseURL := os.Getenv("STRAW_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()

	response, err := straw.NewClient(baseURL, os.Getenv("STRAW_AUTH_TOKEN")).Do(ctx, straw.Request{Method: "GET", URL: "https://example.com", TimeoutMs: requestTimeout})
	if err != nil {
		var apiError *straw.APIError
		if errors.As(err, &apiError) {
			return fmt.Errorf("%w: http=%d code=%s", errAPIRequest, apiError.HTTPStatus, apiError.Response.Code)
		}

		return fmt.Errorf("send request: %w", err)
	}

	fmt.Println(response.Status, response.RequestID)

	return nil
}

// Package cli implements the straw command-line client.
package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"

	"github.com/beremaran/straw-oss/v2/sdk"
)

const (
	defaultBaseURL = "http://localhost:8080"
	usage          = `Straw CLI

Usage:
  straw request --url URL [--method METHOD] [--header "Name: value"] [--body-file PATH]

Environment:
  STRAW_BASE_URL    Control base URL (default http://localhost:8080)
  STRAW_AUTH_TOKEN deployment bearer token (optional for local development)
`
)

var (
	errHeaderFormat = errors.New("header must be Name: value")
	errMethodURL    = errors.New("request requires --url")
	errOpenBody     = errors.New("open body file")
	errUnknownCmd   = errors.New("unknown command")
)

// Run executes the CLI and returns a process exit code.
func Run(ctx context.Context, args []string, _ io.Reader, stdout, stderr io.Writer) int {
	err := run(ctx, args, stdout)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)

		return 1
	}

	return 0
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		_, err := io.WriteString(stdout, usage)
		if err != nil {
			return fmt.Errorf("write usage: %w", err)
		}

		return nil
	}

	if args[0] != "request" {
		return fmt.Errorf("%w %q", errUnknownCmd, args[0])
	}

	return runRequest(ctx, args[1:], stdout)
}

func runRequest(ctx context.Context, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("request", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baseURL := flags.String("base-url", envOr("STRAW_BASE_URL", defaultBaseURL), "Control base URL")
	token := flags.String("token", os.Getenv("STRAW_AUTH_TOKEN"), "deployment bearer token")
	method := flags.String("method", http.MethodGet, "upstream HTTP method")
	targetURL := flags.String("url", "", "absolute upstream URL")
	bodyFile := flags.String("body-file", "", "file to send as the upstream body")
	timeoutMS := flags.Uint64("timeout-ms", 0, "request timeout in milliseconds")
	fingerprint := flags.String("fingerprint-profile", "", "outbound TLS fingerprint profile")

	var headers headerFlags
	flags.Var(&headers, "header", "upstream header as Name: value")

	err := flags.Parse(args)
	if err != nil {
		return fmt.Errorf("parse request flags: %w", err)
	}

	if *targetURL == "" {
		return errMethodURL
	}

	request := sdk.Request{
		Method: *method, URL: *targetURL, TimeoutMs: *timeoutMS,
		FingerprintProfile: *fingerprint,
	}

	request.Headers, err = encodeHeaders(headers)
	if err != nil {
		return err
	}

	if *bodyFile != "" {
		body, readErr := readBody(*bodyFile)
		if readErr != nil {
			return fmt.Errorf("read body file: %w", readErr)
		}

		request.Body = &sdk.RequestBody{Mode: "inline_base64", DataBase64: base64.StdEncoding.EncodeToString(body)}
	}

	response, err := sdk.NewClient(*baseURL, *token).Do(ctx, request)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	err = json.NewEncoder(stdout).Encode(response)
	if err != nil {
		return fmt.Errorf("write response: %w", err)
	}

	return nil
}

func readBody(path string) ([]byte, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)

		return nil, errOpenBody
	}
	defer func() { _ = file.Close() }()

	body, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return body, nil
}

type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ", ") }

func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)

	return nil
}

func encodeHeaders(values []string) ([]sdk.Header, error) {
	headers := make([]sdk.Header, 0, len(values))
	for _, value := range values {
		name, raw, ok := strings.Cut(value, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return nil, errHeaderFormat
		}

		headers = append(headers, sdk.Header{
			Name: strings.TrimSpace(name), ValueBase64: base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(raw))),
		})
	}

	return headers, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req_1","status":200,"body":{"mode":"inline_base64","truncated":false},"timing":{}}`))
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{commandRequest, "--base-url", server.URL, "--token", "secret", "--url", "https://example.com"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), `"status":200`) {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRequestUsesReceipts(t *testing.T) {
	t.Parallel()
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"request_id":"req_1","status":200,"body":{"mode":"receipt","receipt_id":"rcpt_response"},"timing":{}}`))
	}))
	defer server.Close()
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{commandRequest, "--base-url", server.URL, "--method", "POST", "--url", "https://example.com", "--receipt-id", "rcpt_request", "--response-body-mode", cliBodyModeReceipt}, strings.NewReader(""), &stdout, &stderr)
	requestBody, _ := body["body"].(map[string]any)
	if code != 0 || requestBody["receipt_id"] != "rcpt_request" || body["response_body_mode"] != "receipt" {
		t.Fatalf("code=%d body=%#v stderr=%s", code, body, stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"help"}, strings.NewReader(""), &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "straw request") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

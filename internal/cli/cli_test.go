package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/beremaran/straw/v2/sdk"
)

const (
	testCommandAdmin   = "admin"
	testCommandConfig  = "config"
	testCommandRequest = "request"
	testFlagBaseURL    = "--base-url"
	testResourceTenant = "tenants"
)

func TestRequestCommandUsesSDKAndAPIKey(t *testing.T) {
	var got sdk.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/requests" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk_test_123" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		err := json.NewDecoder(r.Body).Decode(&got)
		if err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req_1","status":204,"body":{"mode":"inline_base64"},"timing":{"total_ms":3}}`))
	}))
	defer server.Close()

	var out strings.Builder
	r := runner{stdin: strings.NewReader("body"), stdout: &out, stderr: &strings.Builder{}, httpClient: server.Client()}
	err := r.run(context.Background(), []string{
		testCommandRequest,
		testFlagBaseURL, server.URL,
		"--api-key", "sk_test_123",
		"--method", http.MethodGet,
		"--url", "https://example.test/path",
		"--header", "X-Test: yes",
		"--body-file", "-",
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got.Method != http.MethodGet || got.URL != "https://example.test/path" || !got.Replayable {
		t.Fatalf("request = %+v", got)
	}
	if got.Headers[0].ValueBase64 != base64.StdEncoding.EncodeToString([]byte("yes")) {
		t.Fatalf("header value = %q", got.Headers[0].ValueBase64)
	}
	if got.Body.DataBase64 != base64.StdEncoding.EncodeToString([]byte("body")) {
		t.Fatalf("body = %+v", got.Body)
	}
	if !strings.Contains(out.String(), `"request_id": "req_1"`) {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestConfigUpdateAllowsDocumentedResourcePath(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = string(raw)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	var out strings.Builder
	r := runner{stdin: strings.NewReader(`{"expected_config_version":1}`), stdout: &out, stderr: &strings.Builder{}, httpClient: server.Client()}
	err := r.run(context.Background(), []string{
		testCommandConfig, "update",
		testFlagBaseURL, server.URL,
		"--api-key", "sk_live_secret",
		"--json", "-",
		"routing-rules/route_1",
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/v1/config/routing-rules/route_1" {
		t.Fatalf("%s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer sk_live_secret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody != `{"expected_config_version":1}` {
		t.Fatalf("body = %q", gotBody)
	}
	if out.String() != "{\"ok\":true}\n" {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestRequestCommandLoadsEnvironmentDefaults(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"request_id":"req_env","status":200,"body":{"mode":"inline_base64"},"timing":{}}`))
	}))
	defer server.Close()

	t.Setenv("STRAW_BASE_URL", server.URL)
	t.Setenv("STRAW_API_KEY", "sk_test_env")

	r := runner{stdin: strings.NewReader(""), stdout: &strings.Builder{}, stderr: &strings.Builder{}, httpClient: server.Client()}
	err := r.run(context.Background(), []string{
		testCommandRequest,
		"--method", http.MethodGet,
		"--url", "https://example.test/env",
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if gotAuth != "Bearer sk_test_env" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
}

func TestConfigRejectsUndocumentedResourcePath(t *testing.T) {
	r := runner{stdin: strings.NewReader(`{}`), stdout: &strings.Builder{}, stderr: &strings.Builder{}, httpClient: http.DefaultClient}
	err := r.run(context.Background(), []string{testCommandConfig, "delete", "payload-capture"})
	if err == nil {
		t.Fatal("run() error = nil")
	}
	if !strings.Contains(err.Error(), "unsupported config resource path") {
		t.Fatalf("error = %v", err)
	}
}

func TestAdminCommands(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	r := runner{stdin: strings.NewReader(""), stdout: &strings.Builder{}, stderr: &strings.Builder{}, httpClient: server.Client()}
	for _, args := range [][]string{
		{testCommandAdmin, "workers", testFlagBaseURL, server.URL},
		{testCommandAdmin, "worker", testFlagBaseURL, server.URL, "worker_1", "tenant-drain"},
		{testCommandAdmin, "cancel", testFlagBaseURL, server.URL, "req_1"},
	} {
		err := r.run(context.Background(), args)
		if err != nil {
			t.Fatalf("run(%v) error = %v", args, err)
		}
	}
	want := []string{
		"GET /api/v1/admin/workers",
		"POST /api/v1/admin/workers/worker_1/tenant-drain",
		"POST /api/v1/admin/requests/req_1/cancel",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestRunRedactsSecretsFromErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"bad sk_live_supersecret"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	var stderr strings.Builder
	code := Run(context.Background(), []string{testCommandConfig, "list", testFlagBaseURL, server.URL, testResourceTenant}, strings.NewReader(""), &strings.Builder{}, &stderr)
	if code != 1 {
		t.Fatalf("code = %d", code)
	}
	if strings.Contains(stderr.String(), "supersecret") {
		t.Fatalf("stderr leaked secret: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "sk_live_redacted") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

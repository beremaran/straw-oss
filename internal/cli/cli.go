// Package cli implements the straw command-line client.
package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/beremaran/straw/v2/sdk"
)

const (
	defaultBaseURL = "http://localhost:8080"
	envBaseURL     = "STRAW_BASE_URL"

	commandAdmin             = "admin"
	commandConfig            = "config"
	commandRequest           = "request"
	resourceAPIKey           = "api-keys"
	resourcePlatformAPIKey   = "platform-api-keys"
	resourceRate             = "rate-limits"
	resourceTenant           = "tenants"
	resourceWorkerCredential = "worker-" + "credentials"

	envAPIKey = "STRAW_API_" + "KEY"
	wantPair  = 2
)

var (
	errAPIResponse   = errors.New("api error")
	errHeaderFormat  = errors.New("header must be Name: value")
	errStdinRead     = errors.New("read stdin")
	errStdoutWrite   = errors.New("write stdout")
	errUsageTemplate = errors.New("usage")
)

// Run executes the Straw CLI.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	r := runner{
		stdin:      stdin,
		stdout:     stdout,
		stderr:     stderr,
		httpClient: http.DefaultClient,
	}

	err := r.run(ctx, args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, redact(err.Error()))

		return 1
	}

	return 0
}

type runner struct {
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	httpClient *http.Client
}

func (r runner) run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("missing command")
	}

	switch args[0] {
	case commandRequest:
		return r.request(ctx, args[1:])
	case commandConfig:
		return r.config(ctx, args[1:])
	case commandAdmin:
		return r.admin(ctx, args[1:])
	case "healthz", "readyz", "metrics":
		return r.operational(ctx, args)
	case "help", "-h", "--help":
		_, err := fmt.Fprint(r.stdout, usage)
		if err != nil {
			return fmt.Errorf("write usage: %w", err)
		}

		return nil
	default:
		return usageError("unknown command %q", args[0])
	}
}

func (r runner) operational(ctx context.Context, args []string) error {
	fs := newFlagSet(args[0])

	baseURL := fs.String("base-url", envOrDefault(envBaseURL, defaultBaseURL), "Control API base URL")

	err := fs.Parse(args[1:])
	if err != nil {
		return fmt.Errorf("parse %s flags: %w", args[0], err)
	}

	if fs.NArg() != 0 {
		return usageError("%s takes no positional arguments", args[0])
	}

	return r.do(ctx, *baseURL, "", http.MethodGet, "/"+args[0], nil)
}

func (r runner) request(ctx context.Context, args []string) error {
	fs := newFlagSet("request")
	baseURL := fs.String("base-url", envOrDefault(envBaseURL, defaultBaseURL), "Control API base URL")
	apiKey := fs.String("api-key", os.Getenv(envAPIKey), "API key")
	jsonPath := fs.String("json", "", "request JSON file, or - for stdin")
	method := fs.String("method", "", "HTTP method")
	targetURL := fs.String("url", "", "absolute upstream URL")
	bodyFile := fs.String("body-file", "", "request body file")
	timeoutMS := fs.Uint64("timeout-ms", 0, "request timeout in milliseconds")
	fingerprint := fs.String("fingerprint-profile", "", "fingerprint profile")
	captureHint := fs.String("capture-hint", "", "capture hint")

	var headers headerFlags
	fs.Var(&headers, "header", "HTTP header as Name: value")

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse request flags: %w", err)
	}

	if fs.NArg() != 0 {
		return usageError("request takes no positional arguments")
	}

	req, err := r.requestEnvelope(*jsonPath, *method, *targetURL, *timeoutMS, *fingerprint, *captureHint, *bodyFile, headers)
	if err != nil {
		return err
	}

	if req.Method == "" || req.URL == "" {
		return usageError("request needs --method and --url, or --json containing both")
	}

	resp, err := sdk.NewClient(*baseURL, *apiKey, sdk.WithHTTPClient(r.httpClient)).Do(ctx, req)
	if err != nil {
		return fmt.Errorf("submit request: %w", err)
	}

	return writeJSON(r.stdout, resp)
}

func (r runner) config(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("missing config command")
	}

	switch args[0] {
	case "list":
		return r.configList(ctx, args[1:])
	case "get":
		return r.configGet(ctx, args[1:])
	case "create":
		return r.configBody(ctx, http.MethodPost, configCreate, args[1:])
	case "update":
		return r.configBody(ctx, http.MethodPut, configUpdate, args[1:])
	case "delete":
		return r.configNoBody(ctx, http.MethodDelete, configDelete, args[1:])
	case "revoke":
		return r.configRevoke(ctx, args[1:])
	case "rollback":
		return r.configRollback(ctx, args[1:])
	default:
		return usageError("unknown config command %q", args[0])
	}
}

func (r runner) configList(ctx context.Context, args []string) error {
	fs := newFlagSet("config list")
	baseURL, apiKey := commonFlags(fs)
	limit := fs.Int("limit", 0, "page size")

	offset := fs.Int("offset", 0, "page offset")

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse config list flags: %w", err)
	}

	if fs.NArg() != 1 {
		return usageError("config list needs a resource")
	}

	resource := fs.Arg(0)
	if !configList[resource] {
		return usageError("unsupported config list resource %q", resource)
	}

	path := "/api/v1/config/" + resource

	values := url.Values{}
	if *limit > 0 {
		values.Set("limit", strconv.Itoa(*limit))
	}

	if *offset > 0 {
		values.Set("offset", strconv.Itoa(*offset))
	}

	if qs := values.Encode(); qs != "" {
		path += "?" + qs
	}

	return r.do(ctx, *baseURL, *apiKey, http.MethodGet, path, nil)
}

func (r runner) configGet(ctx context.Context, args []string) error {
	fs := newFlagSet("config get")

	baseURL, apiKey := commonFlags(fs)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse config get flags: %w", err)
	}

	path, err := configGetPath(fs.Args())
	if err != nil {
		return err
	}

	return r.do(ctx, *baseURL, *apiKey, http.MethodGet, path, nil)
}

func (r runner) configBody(ctx context.Context, method string, allowed map[string]bool, args []string) error {
	fs := newFlagSet("config body")
	baseURL, apiKey := commonFlags(fs)

	jsonPath := fs.String("json", "-", "JSON body file, or - for stdin")

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse config body flags: %w", err)
	}

	if fs.NArg() != 1 {
		return usageError("config command needs one resource path")
	}

	resource := fs.Arg(0)
	if !allowedPath(allowed, resource) {
		return usageError("unsupported config resource path %q", resource)
	}

	raw, err := r.readInput(*jsonPath)
	if err != nil {
		return err
	}

	return r.do(ctx, *baseURL, *apiKey, method, "/api/v1/config/"+resource, raw)
}

func (r runner) configNoBody(ctx context.Context, method string, allowed map[string]bool, args []string) error {
	fs := newFlagSet("config delete")

	baseURL, apiKey := commonFlags(fs)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse config delete flags: %w", err)
	}

	if fs.NArg() != 1 {
		return usageError("config delete needs one resource path")
	}

	resource := fs.Arg(0)
	if !allowedPath(allowed, resource) {
		return usageError("unsupported config resource path %q", resource)
	}

	return r.do(ctx, *baseURL, *apiKey, method, "/api/v1/config/"+resource, nil)
}

func (r runner) configRevoke(ctx context.Context, args []string) error {
	fs := newFlagSet("config revoke")

	baseURL, apiKey := commonFlags(fs)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse config revoke flags: %w", err)
	}

	if fs.NArg() != wantPair {
		return usageError("config revoke needs resource and id")
	}

	resource, id := fs.Arg(0), fs.Arg(1)
	if !configRevoke[resource] {
		return usageError("unsupported revoke resource %q", resource)
	}

	return r.do(ctx, *baseURL, *apiKey, http.MethodPost, "/api/v1/config/"+resource+"/"+id+"/revoke", nil)
}

func (r runner) configRollback(ctx context.Context, args []string) error {
	fs := newFlagSet("config rollback")
	baseURL, apiKey := commonFlags(fs)

	jsonPath := fs.String("json", "-", "rollback JSON file, or - for stdin")

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse config rollback flags: %w", err)
	}

	if fs.NArg() != 0 {
		return usageError("config rollback takes no positional arguments")
	}

	raw, err := r.readInput(*jsonPath)
	if err != nil {
		return err
	}

	return r.do(ctx, *baseURL, *apiKey, http.MethodPost, "/api/v1/config/rollback", raw)
}

func (r runner) admin(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return usageError("missing admin command")
	}

	switch args[0] {
	case "workers":
		return r.adminWorkers(ctx, args[1:])
	case "worker":
		return r.adminWorkerAction(ctx, args[1:])
	case "cancel":
		return r.adminCancel(ctx, args[1:])
	default:
		return usageError("unknown admin command %q", args[0])
	}
}

func (r runner) adminWorkers(ctx context.Context, args []string) error {
	fs := newFlagSet("admin workers")

	baseURL, apiKey := commonFlags(fs)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse admin workers flags: %w", err)
	}

	if fs.NArg() != 0 {
		return usageError("admin workers takes no positional arguments")
	}

	return r.do(ctx, *baseURL, *apiKey, http.MethodGet, "/api/v1/admin/workers", nil)
}

func (r runner) adminWorkerAction(ctx context.Context, args []string) error {
	fs := newFlagSet("admin worker")

	baseURL, apiKey := commonFlags(fs)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse admin worker flags: %w", err)
	}

	if fs.NArg() != wantPair {
		return usageError("admin worker needs worker_id and action")
	}

	workerID, action := fs.Arg(0), fs.Arg(1)
	if !workerActions[action] {
		return usageError("unsupported worker action %q", action)
	}

	return r.do(ctx, *baseURL, *apiKey, http.MethodPost, "/api/v1/admin/workers/"+workerID+"/"+action, nil)
}

func (r runner) adminCancel(ctx context.Context, args []string) error {
	fs := newFlagSet("admin cancel")

	baseURL, apiKey := commonFlags(fs)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("parse admin cancel flags: %w", err)
	}

	if fs.NArg() != 1 {
		return usageError("admin cancel needs request_id")
	}

	return r.do(ctx, *baseURL, *apiKey, http.MethodPost, "/api/v1/admin/requests/"+fs.Arg(0)+"/cancel", nil)
}

func (r runner) do(ctx context.Context, baseURL, apiKey, method, path string, body []byte) error {
	u := strings.TrimRight(baseURL, "/") + path

	req, err := buildRequest(ctx, method, u, apiKey, body)
	if err != nil {
		return err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	return r.writeResponse(resp)
}

func buildRequest(ctx context.Context, method, rawURL, apiKey string, body []byte) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	return req, nil
}

func (r runner) writeResponse(resp *http.Response) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w %d: %s", errAPIResponse, resp.StatusCode, string(raw))
	}

	if len(raw) == 0 {
		_, err = fmt.Fprintln(r.stdout, "{}")
		if err != nil {
			return fmt.Errorf("%w: %w", errStdoutWrite, err)
		}

		return nil
	}

	_, err = r.stdout.Write(append(bytes.TrimSpace(raw), '\n'))
	if err != nil {
		return fmt.Errorf("%w: %w", errStdoutWrite, err)
	}

	return nil
}

func (r runner) readInput(path string) ([]byte, error) {
	if path == "-" {
		raw, err := io.ReadAll(r.stdin)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errStdinRead, err)
		}

		return raw, nil
	}

	fd, err := syscall.Open(path, syscall.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		_ = syscall.Close(fd)

		return nil, fmt.Errorf("read %s: %w", path, errStdinRead)
	}

	defer func() {
		_ = f.Close()
	}()

	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return raw, nil
}

func configGetPath(args []string) (string, error) {
	if len(args) == 1 {
		switch args[0] {
		case "quotas", resourceRate, "fingerprint-profiles", "changes":
			return "/api/v1/config/" + args[0], nil
		}
	}

	if len(args) == wantPair && args[0] == resourceTenant {
		return "/api/v1/config/tenants/" + args[1], nil
	}

	return "", usageError("unsupported config get target")
}

func commonFlags(fs *flag.FlagSet) (*string, *string) {
	return fs.String("base-url", envOrDefault(envBaseURL, defaultBaseURL), "Control API base URL"),
		fs.String("api-key", os.Getenv(envAPIKey), "API key")
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	return fs
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	err := enc.Encode(v)
	if err != nil {
		return fmt.Errorf("%w: %w", errStdoutWrite, err)
	}

	return nil
}

type headerFlags []sdk.Header

func (h *headerFlags) String() string { return "" }

func (h *headerFlags) Set(value string) error {
	name, raw, ok := strings.Cut(value, ":")
	if !ok || strings.TrimSpace(name) == "" {
		return errHeaderFormat
	}

	*h = append(*h, sdk.Header{
		Name:        strings.TrimSpace(name),
		ValueBase64: base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(raw))),
	})

	return nil
}

func usageError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUsageTemplate, fmt.Sprintf(format, args...))
}

func redact(s string) string {
	var b strings.Builder

	for i := 0; i < len(s); {
		marker := secretMarkerAt(s[i:])
		if marker == "" {
			b.WriteByte(s[i])
			i++

			continue
		}

		b.WriteString(marker)
		b.WriteString("redacted")

		i += len(marker)
		for i < len(s) && isSecretChar(s[i]) {
			i++
		}
	}

	return b.String()
}

func secretMarkerAt(s string) string {
	for _, marker := range []string{"sk_live_", "sk_test_"} {
		if strings.HasPrefix(s, marker) {
			return marker
		}
	}

	return ""
}

func isSecretChar(b byte) bool {
	return b == '_' || b == '-' || b == '.' || b == '~' ||
		('0' <= b && b <= '9') ||
		('a' <= b && b <= 'z') ||
		('A' <= b && b <= 'Z')
}

var configList = map[string]bool{
	resourceTenant:           true,
	resourcePlatformAPIKey:   true,
	resourceAPIKey:           true,
	resourceWorkerCredential: true,
	"executor-pools":         true,
	"routing-rules":          true,
	"fingerprint-profiles":   true,
	"injection-policies":     true,
	"quotas":                 true,
	resourceRate:             true,
	"deny-rules":             true,
	"changes":                true,
}

var configCreate = map[string]bool{
	resourceTenant:           true,
	"tenants/{id}/api-keys":  true,
	resourcePlatformAPIKey:   true,
	resourceAPIKey:           true,
	resourceWorkerCredential: true,
	"executor-pools":         true,
	"routing-rules":          true,
	"injection-policies":     true,
	"deny-rules":             true,
}

var configUpdate = map[string]bool{
	resourceRate: true,
}

var configDelete = map[string]bool{}

var configRevoke = map[string]bool{
	resourcePlatformAPIKey:   true,
	resourceAPIKey:           true,
	resourceWorkerCredential: true,
}

var workerActions = map[string]bool{
	"disable":        true,
	"enable":         true,
	"drain":          true,
	"undrain":        true,
	"tenant-disable": true,
	"tenant-enable":  true,
	"tenant-drain":   true,
	"tenant-undrain": true,
}

func init() {
	for _, pattern := range []string{"tenants/{id}", "executor-pools/{id}", "routing-rules/{id}", "injection-policies/{id}", "deny-rules/{id}"} {
		configDelete[pattern] = true
	}

	for _, pattern := range []string{"tenants/{id}", "executor-pools/{id}", "routing-rules/{id}", "injection-policies/{id}", "deny-rules/{id}"} {
		configUpdate[pattern] = true
	}

	configUpdate["tenants/{id}/quotas"] = true
}

func allowedPath(patterns map[string]bool, path string) bool {
	if patterns[path] {
		return true
	}

	pathParts := strings.Split(path, "/")
	for pattern := range patterns {
		patternParts := strings.Split(pattern, "/")
		if len(patternParts) != len(pathParts) {
			continue
		}

		ok := true

		for i := range patternParts {
			if patternParts[i] == "{id}" {
				ok = ok && pathParts[i] != ""

				continue
			}

			if patternParts[i] != pathParts[i] {
				ok = false

				break
			}
		}

		if ok {
			return true
		}
	}

	return false
}

const usage = `Usage:
  straw request --method GET --url https://example.com [--header 'Name: value'] [--body-file path]
  straw request --json request.json
  straw config list [--limit n] [--offset n] <resource>
  straw config get tenants <id>|quotas|rate-limits|fingerprint-profiles|changes
  straw config create --json body.json <resource>
  straw config update --json body.json <resource-path>
  straw config delete <resource-path>
  straw config revoke platform-api-keys|api-keys|worker-credentials <id>
  straw config rollback --json body.json
  straw admin workers
  straw admin worker <worker_id> disable|enable|drain|undrain|tenant-disable|tenant-enable|tenant-drain|tenant-undrain
  straw admin cancel <request_id>
  straw healthz|readyz|metrics

Environment:
  STRAW_BASE_URL  Control API base URL, default http://localhost:8080
  STRAW_API_KEY   API key used as Bearer token
`

func (r runner) requestEnvelope(jsonPath, method, targetURL string, timeoutMS uint64, fingerprint, captureHint, bodyFile string, headers headerFlags) (sdk.Request, error) {
	req := sdk.Request{}

	if jsonPath != "" {
		raw, err := r.readInput(jsonPath)
		if err != nil {
			return sdk.Request{}, err
		}

		err = json.Unmarshal(raw, &req)
		if err != nil {
			return sdk.Request{}, fmt.Errorf("decode request JSON: %w", err)
		}
	}

	applyRequestFlags(&req, method, targetURL, timeoutMS, fingerprint, captureHint, headers)

	if bodyFile == "" {
		return req, nil
	}

	raw, err := r.readInput(bodyFile)
	if err != nil {
		return sdk.Request{}, err
	}

	req.Body = &sdk.RequestBody{Mode: "inline_base64", DataBase64: base64.StdEncoding.EncodeToString(raw)}

	return req, nil
}

func applyRequestFlags(req *sdk.Request, method, targetURL string, timeoutMS uint64, fingerprint, captureHint string, headers headerFlags) {
	if method != "" {
		req.Method = method
	}

	if targetURL != "" {
		req.URL = targetURL
	}

	if timeoutMS != 0 {
		req.TimeoutMs = timeoutMS
	}

	if fingerprint != "" {
		req.FingerprintProfile = fingerprint
	}

	if captureHint != "" {
		req.CaptureHint = captureHint
	}

	if len(headers) > 0 {
		req.Headers = append(req.Headers, headers...)
	}
}

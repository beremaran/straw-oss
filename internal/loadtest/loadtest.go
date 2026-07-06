// Package loadtest contains the small local load harness used by task P1-18.
package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultP50CoordinationSLO is the documented Control routing plus assignment p50 target.
	DefaultP50CoordinationSLO = 100 * time.Millisecond
	// DefaultP99CoordinationSLO is the documented Control routing plus assignment p99 target.
	DefaultP99CoordinationSLO = 500 * time.Millisecond

	defaultRequestTimeout    = 30 * time.Second
	defaultClickHouseTimeout = 10 * time.Second
	percentileP50            = 50
	percentileP99            = 99
	percentScale             = 100
	requestTimeoutMS         = 15000
)

var (
	errRequestsRequired          = errors.New("requests must be > 0")
	errConcurrencyRequired       = errors.New("concurrency must be > 0")
	errRunConfigRequired         = errors.New("base URL, API key, and target URL are required")
	errTimingExceedsTotal        = errors.New("phase timings exceed total")
	errNoCompletedRequests       = errors.New("no completed requests")
	errUnexpectedFailures        = errors.New("requests failed unexpectedly")
	errExpectedFailuresMissing   = errors.New("expected at least one capacity/backpressure rejection")
	errP50Exceeded               = errors.New("coordination p50 exceeds SLO")
	errP99Exceeded               = errors.New("coordination p99 exceeds SLO")
	errClickHouseRowsMismatch    = errors.New("ClickHouse request metadata row mismatch")
	errClickHouseEndpointMissing = errors.New("ClickHouse endpoint is required")
	errClickHouseQueryFailed     = errors.New("query ClickHouse failed")
)

// Config defines one load-harness run.
type Config struct {
	BaseURL        string
	APIKey         string
	TargetURL      string
	Requests       int
	Concurrency    int
	HTTPClient     *http.Client
	ExpectFailures bool
}

// Result contains all samples from a load-harness run.
type Result struct {
	Samples []Sample
	Started time.Time
	Ended   time.Time
}

// Sample is the observed result for one request.
type Sample struct {
	RequestID    string
	Status       int
	ErrorCode    string
	RoutingMS    int64
	AssignmentMS int64
	EgressMS     int64
	TotalMS      int64
	Elapsed      time.Duration
	Err          string
}

// Limits defines pass/fail checks for a load run.
type Limits struct {
	P50Coordination time.Duration
	P99Coordination time.Duration
	ExpectFailures  bool
	AssertRows      bool
	CompletedRows   int
}

// Run executes the configured requests against Control's REST request endpoint.
func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Requests <= 0 {
		return Result{}, errRequestsRequired
	}

	if cfg.Concurrency <= 0 {
		return Result{}, errConcurrencyRequired
	}

	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.TargetURL == "" {
		return Result{}, errRunConfigRequired
	}

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeout}
	}

	result := Result{Samples: make([]Sample, cfg.Requests), Started: time.Now()}
	jobs := make(chan int)

	var wg sync.WaitGroup

	for range cfg.Concurrency {
		wg.Go(func() {
			for i := range jobs {
				result.Samples[i] = runOne(ctx, client, cfg)
			}
		})
	}

	for i := range cfg.Requests {
		jobs <- i
	}

	close(jobs)
	wg.Wait()

	result.Ended = time.Now()

	return result, nil
}

// Evaluate checks one run against the SLO, rejection, timing, and optional ClickHouse row assertions.
func Evaluate(result Result, limits Limits) error {
	if limits.P50Coordination == 0 {
		limits.P50Coordination = DefaultP50CoordinationSLO
	}

	if limits.P99Coordination == 0 {
		limits.P99Coordination = DefaultP99CoordinationSLO
	}

	completed, failures, coordination, err := collectSamples(result)
	if err != nil {
		return err
	}

	if completed == 0 {
		return errNoCompletedRequests
	}

	err = checkFailureExpectation(failures, limits.ExpectFailures)
	if err != nil {
		return err
	}

	err = checkCoordinationSLO(coordination, limits)
	if err != nil {
		return err
	}

	return checkClickHouseRows(completed, limits)
}

// Summary returns one line of load-run counters and coordination percentiles.
func Summary(result Result) string {
	completed := 0
	failures := 0

	var coordination []time.Duration

	for _, s := range result.Samples {
		if s.Err != "" || s.ErrorCode != "" {
			failures++

			continue
		}

		completed++

		coordination = append(coordination, time.Duration(s.RoutingMS+s.AssignmentMS)*time.Millisecond)
	}

	return fmt.Sprintf("requests=%d completed=%d failures=%d elapsed=%s coordination_p50=%s coordination_p99=%s",
		len(result.Samples), completed, failures, result.Ended.Sub(result.Started).Round(time.Millisecond),
		percentile(coordination, percentileP50), percentile(coordination, percentileP99))
}

// CompletedRequestIDs returns successful request IDs for ClickHouse row checks.
func CompletedRequestIDs(result Result) []string {
	ids := make([]string, 0, len(result.Samples))

	for _, s := range result.Samples {
		if s.RequestID != "" && s.Err == "" && s.ErrorCode == "" {
			ids = append(ids, s.RequestID)
		}
	}

	return ids
}

// CountClickHouseRequestMetadataRows counts matching canonical request_events rows in a live ClickHouse database.
func CountClickHouseRequestMetadataRows(ctx context.Context, client *http.Client, endpoint, database string, requestIDs []string) (int, error) {
	if client == nil {
		client = &http.Client{Timeout: defaultClickHouseTimeout}
	}

	if endpoint == "" {
		return 0, errClickHouseEndpointMissing
	}

	if database == "" {
		database = "straw"
	}

	if len(requestIDs) == 0 {
		return 0, nil
	}

	query := fmt.Sprintf("SELECT count() FROM %s.request_events WHERE request_id IN (%s) FORMAT TabSeparated",
		clickHouseIdent(database), clickHouseStrings(requestIDs))

	body, err := doClickHouseQuery(ctx, client, endpoint, query)
	if err != nil {
		return 0, err
	}

	var count int

	_, err = fmt.Sscanf(string(body), "%d", &count)
	if err != nil {
		return 0, fmt.Errorf("parse ClickHouse count %q: %w", strings.TrimSpace(string(body)), err)
	}

	return count, nil
}

func collectSamples(result Result) (int, int, []time.Duration, error) {
	var coordination []time.Duration

	failures := 0
	completed := 0

	for _, s := range result.Samples {
		if s.Err != "" || s.ErrorCode != "" {
			failures++

			continue
		}

		if s.TotalMS < s.RoutingMS+s.AssignmentMS+s.EgressMS {
			return 0, 0, nil, fmt.Errorf("%w: request %s", errTimingExceedsTotal, s.RequestID)
		}

		completed++

		coordination = append(coordination, time.Duration(s.RoutingMS+s.AssignmentMS)*time.Millisecond)
	}

	return completed, failures, coordination, nil
}

func checkFailureExpectation(failures int, expectFailures bool) error {
	if failures > 0 && !expectFailures {
		return fmt.Errorf("%w: %d", errUnexpectedFailures, failures)
	}

	if failures == 0 && expectFailures {
		return errExpectedFailuresMissing
	}

	return nil
}

func checkCoordinationSLO(coordination []time.Duration, limits Limits) error {
	p50 := percentile(coordination, percentileP50)
	p99 := percentile(coordination, percentileP99)

	if p50 > limits.P50Coordination {
		return fmt.Errorf("%w: %s > %s", errP50Exceeded, p50, limits.P50Coordination)
	}

	if p99 > limits.P99Coordination {
		return fmt.Errorf("%w: %s > %s", errP99Exceeded, p99, limits.P99Coordination)
	}

	return nil
}

func checkClickHouseRows(completed int, limits Limits) error {
	if limits.AssertRows && limits.CompletedRows != completed {
		return fmt.Errorf("%w: rows=%d completed=%d", errClickHouseRowsMismatch, limits.CompletedRows, completed)
	}

	return nil
}

func doClickHouseQuery(ctx context.Context, client *http.Client, endpoint, query string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/", strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("build ClickHouse request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query ClickHouse: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read ClickHouse response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: %s: %s", errClickHouseQueryFailed, resp.Status, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func runOne(ctx context.Context, client *http.Client, cfg Config) Sample {
	body := fmt.Appendf(nil, `{"method":"GET","url":%q,"timeout_ms":%d}`, cfg.TargetURL, requestTimeoutMS)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.BaseURL, "/")+"/api/v1/requests", bytes.NewReader(body))
	if err != nil {
		return Sample{Err: err.Error()}
	}

	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()

	resp, err := client.Do(req)

	elapsed := time.Since(started)
	if err != nil {
		return Sample{Elapsed: elapsed, Err: err.Error()}
	}

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Sample{Elapsed: elapsed, Err: err.Error()}
	}

	var success responseEnvelope

	if resp.StatusCode == http.StatusOK && json.Unmarshal(raw, &success) == nil && success.RequestID != "" {
		return Sample{
			RequestID:    success.RequestID,
			Status:       success.Status,
			RoutingMS:    success.Timing.RoutingMS,
			AssignmentMS: success.Timing.AssignmentMS,
			EgressMS:     success.Timing.EgressMS,
			TotalMS:      success.Timing.TotalMS,
			Elapsed:      elapsed,
		}
	}

	var failure errorEnvelope

	_ = json.Unmarshal(raw, &failure)

	return Sample{RequestID: failure.RequestID, ErrorCode: failure.Code, Elapsed: elapsed, Err: failure.Message}
}

type responseEnvelope struct {
	RequestID string `json:"request_id"`
	Status    int    `json:"status"`
	Timing    struct {
		RoutingMS    int64 `json:"routing_ms"`
		AssignmentMS int64 `json:"assignment_ms"`
		EgressMS     int64 `json:"egress_ms"`
		TotalMS      int64 `json:"total_ms"`
	} `json:"timing"`
}

type errorEnvelope struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

func percentile(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]time.Duration(nil), values...)

	slices.Sort(sorted)

	idx := min(len(sorted), max(1, (len(sorted)*p+percentScale-1)/percentScale))

	return sorted[idx-1]
}

func clickHouseIdent(s string) string {
	var b strings.Builder

	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}

	if b.Len() == 0 {
		return "straw"
	}

	return b.String()
}

func clickHouseStrings(values []string) string {
	quoted := make([]string, 0, len(values))

	for _, v := range values {
		quoted = append(quoted, "'"+strings.ReplaceAll(v, "'", "''")+"'")
	}

	return strings.Join(quoted, ",")
}

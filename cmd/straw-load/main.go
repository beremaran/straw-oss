// Command straw-load runs Straw's local load and ClickHouse row-count harness.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/beremaran/straw/v2/internal/loadtest"
)

const (
	defaultRequests       = 20
	defaultConcurrency    = 4
	defaultClickHouseWait = 2 * time.Second
	defaultHTTPTimeout    = 30 * time.Second
)

type cliConfig struct {
	run            loadtest.Config
	clickHouseURL  string
	clickHouseDB   string
	clickHouseWait time.Duration
	p50            time.Duration
	p99            time.Duration
}

func main() {
	cfg := parseFlags()

	err := run(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() cliConfig {
	var cfg cliConfig

	flag.StringVar(&cfg.run.BaseURL, "base-url", "http://localhost:8080", "Control API base URL")
	flag.StringVar(&cfg.run.APIKey, "api-key", os.Getenv("STRAW_LOAD_API_KEY"), "tenant requester API key")
	flag.StringVar(&cfg.run.TargetURL, "target-url", "https://example.com/", "upstream URL")
	flag.IntVar(&cfg.run.Requests, "requests", defaultRequests, "number of requests")
	flag.IntVar(&cfg.run.Concurrency, "concurrency", defaultConcurrency, "concurrent requests")
	flag.BoolVar(&cfg.run.ExpectFailures, "expect-rejections", false, "require at least one capacity/backpressure rejection")
	flag.DurationVar(&cfg.p50, "coordination-p50", loadtest.DefaultP50CoordinationSLO, "routing+assignment p50 SLO")
	flag.DurationVar(&cfg.p99, "coordination-p99", loadtest.DefaultP99CoordinationSLO, "routing+assignment p99 SLO")
	flag.StringVar(&cfg.clickHouseURL, "clickhouse-url", "", "optional ClickHouse HTTP URL for request_events row assertion")
	flag.StringVar(&cfg.clickHouseDB, "clickhouse-db", "straw", "ClickHouse database")
	flag.DurationVar(&cfg.clickHouseWait, "clickhouse-wait", defaultClickHouseWait, "wait before querying async ClickHouse rows")
	flag.Parse()

	return cfg
}

func run(cfg cliConfig) error {
	ctx := context.Background()
	cfg.run.HTTPClient = &http.Client{Timeout: defaultHTTPTimeout}

	result, err := loadtest.Run(ctx, cfg.run)
	if err != nil {
		return fmt.Errorf("run load test: %w", err)
	}

	rows, err := maybeCountClickHouseRows(ctx, cfg, result)
	if err != nil {
		return err
	}

	err = loadtest.Evaluate(result, loadtest.Limits{
		P50Coordination: cfg.p50,
		P99Coordination: cfg.p99,
		ExpectFailures:  cfg.run.ExpectFailures,
		AssertRows:      cfg.clickHouseURL != "",
		CompletedRows:   rows,
	})

	fmt.Println(loadtest.Summary(result))

	if rows > 0 {
		fmt.Printf("clickhouse_request_metadata_rows=%d\n", rows)
	}

	if err != nil {
		return fmt.Errorf("evaluate load test: %w", err)
	}

	return nil
}

func maybeCountClickHouseRows(ctx context.Context, cfg cliConfig, result loadtest.Result) (int, error) {
	if cfg.clickHouseURL == "" {
		return 0, nil
	}

	time.Sleep(cfg.clickHouseWait)

	rows, err := loadtest.CountClickHouseRequestMetadataRows(ctx, cfg.run.HTTPClient, cfg.clickHouseURL, cfg.clickHouseDB, loadtest.CompletedRequestIDs(result))
	if err != nil {
		return 0, fmt.Errorf("count ClickHouse rows: %w", err)
	}

	return rows, nil
}

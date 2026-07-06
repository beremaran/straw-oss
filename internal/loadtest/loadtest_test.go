package loadtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testRequestID1 = "req_1"
	testRequestID2 = "req_2"
)

func TestEvaluateChecksCoordinationSLOAndRows(t *testing.T) {
	t.Parallel()

	result := Result{
		Samples: []Sample{
			{RequestID: testRequestID1, RoutingMS: 10, AssignmentMS: 20, EgressMS: 30, TotalMS: 70},
			{RequestID: testRequestID2, RoutingMS: 20, AssignmentMS: 30, EgressMS: 40, TotalMS: 100},
		},
		Started: time.Unix(0, 0),
		Ended:   time.Unix(0, int64(time.Second)),
	}

	err := Evaluate(result, Limits{P50Coordination: 60 * time.Millisecond, P99Coordination: 60 * time.Millisecond, AssertRows: true, CompletedRows: 2})
	if err != nil {
		t.Fatalf("Evaluate() error = %v, want nil", err)
	}
}

func TestEvaluateRejectsOverclaimedSLO(t *testing.T) {
	t.Parallel()

	result := Result{Samples: []Sample{{RequestID: testRequestID1, RoutingMS: 400, AssignmentMS: 200, TotalMS: 700}}}

	err := Evaluate(result, Limits{P50Coordination: 100 * time.Millisecond, P99Coordination: 500 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "coordination p50") {
		t.Fatalf("Evaluate() error = %v, want p50 violation", err)
	}
}

func TestEvaluateRequiresExpectedRejections(t *testing.T) {
	t.Parallel()

	result := Result{Samples: []Sample{{RequestID: testRequestID1, RoutingMS: 1, AssignmentMS: 1, TotalMS: 3}}}

	err := Evaluate(result, Limits{ExpectFailures: true})
	if err == nil || !strings.Contains(err.Error(), "expected at least one") {
		t.Fatalf("Evaluate() error = %v, want missing rejection", err)
	}
}

func TestEvaluateChecksClickHouseRequestMetadataRows(t *testing.T) {
	t.Parallel()

	result := Result{Samples: []Sample{
		{RequestID: testRequestID1, RoutingMS: 1, AssignmentMS: 1, TotalMS: 3},
		{RequestID: testRequestID2, RoutingMS: 1, AssignmentMS: 1, TotalMS: 3},
	}}

	err := Evaluate(result, Limits{AssertRows: true, CompletedRows: 1})
	if err == nil || !strings.Contains(err.Error(), "ClickHouse request metadata row mismatch") {
		t.Fatalf("Evaluate() error = %v, want row-count mismatch", err)
	}
}

func TestCountClickHouseRequestMetadataRowsQueriesRequestEvents(t *testing.T) {
	t.Parallel()

	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		gotQuery = string(raw)
		_, _ = w.Write([]byte("2\n"))
	}))
	defer srv.Close()

	count, err := CountClickHouseRequestMetadataRows(context.Background(), srv.Client(), srv.URL, "straw", []string{testRequestID1, testRequestID2})
	if err != nil {
		t.Fatalf("CountClickHouseRequestMetadataRows() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if !strings.Contains(gotQuery, "FROM straw.request_events") || !strings.Contains(gotQuery, "'req_1','req_2'") {
		t.Fatalf("query = %q, want request_events IN query", gotQuery)
	}
}

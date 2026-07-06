package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	telemetryTestSessionID        = "session_id"
	telemetryTestSelectedExecutor = "selected_executor"
	telemetryTestWorker1          = "worker_1"
)

func TestTelemetryRequestsScopesTenantAndRedactsTopology(t *testing.T) {
	t.Parallel()

	h, token, store := newTestTelemetryHandlers(t, RoleViewer)
	store.requests = []RequestTelemetryItem{{
		Timestamp: "2026-07-06T12:00:00.123Z", RequestID: requestMetadataTestRequestID, APIKeyRef: "key_pub",
		TargetURL: "https://example.com/path", AttemptCount: 2,
	}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/requests?from=2026-07-06T11:00:00Z&to=2026-07-06T12:00:00Z&limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.Requests(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if store.last.TenantID != adminTestTenantA {
		t.Fatalf("tenant = %q, want authenticated tenant", store.last.TenantID)
	}
	body := w.Body.String()
	for _, forbidden := range []string{telemetryFieldWorkerID, telemetryTestSessionID, telemetryTestSelectedExecutor} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response leaked %s: %s", forbidden, body)
		}
	}
	var resp telemetryEnvelope[RequestTelemetryItem]
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Items[0].AttemptCount != 2 {
		t.Fatalf("attempt_count = %d, want 2", resp.Items[0].AttemptCount)
	}
	if resp.Query.From != "2026-07-06T11:00:00.000Z" || resp.Query.To != "2026-07-06T12:00:00.000Z" {
		t.Fatalf("query timestamps = %q/%q, want millisecond RFC3339", resp.Query.From, resp.Query.To)
	}
}

func TestTelemetryTimeUsesMilliseconds(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 7, 6, 12, 0, 0, 123456789, time.UTC)
	if got := telemetryTime(ts); got != "2026-07-06T12:00:00.123Z" {
		t.Fatalf("telemetryTime() = %q, want milliseconds", got)
	}
}

func TestTelemetryRejectsTenantOverrideAndWideWindow(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestTelemetryHandlers(t, RoleViewer)
	for _, path := range []string{
		"/api/v1/telemetry/requests?tenant_id=ten_b",
		"/api/v1/telemetry/requests?from=2026-07-01T00:00:00Z&to=2026-07-06T00:00:00Z",
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		h.Requests(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", path, w.Code)
		}
	}
}

func TestTelemetryRejectsRawNumericFilterSQL(t *testing.T) {
	t.Parallel()

	h, token, store := newTestTelemetryHandlers(t, RoleViewer)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/requests?client_status=200%20OR%201=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.Requests(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if store.last.Filters != nil {
		t.Fatalf("store was queried with filters = %#v", store.last.Filters)
	}
}

func TestTelemetryRequesterOnlyGetsOwnRequestDetail(t *testing.T) {
	t.Parallel()

	h, token, store := newTestTelemetryHandlers(t, RoleRequester)
	store.detail = RequestTelemetryDetail{RequestTelemetryItem: RequestTelemetryItem{RequestID: requestMetadataTestRequestID}, Attempts: []RequestTelemetryAttempt{{Attempt: 1}}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/requests/"+requestMetadataTestRequestID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.RequestDetail(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if store.detailTenant != adminTestTenantA {
		t.Fatalf("detail tenant = %q, want ten_a", store.detailTenant)
	}
}

func TestTelemetryStoreErrorsMapToPublicResponses(t *testing.T) {
	t.Parallel()

	h, token, store := newTestTelemetryHandlers(t, RoleViewer)
	store.err = errTelemetryReadLimit
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/audit", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.Audit(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("read-limit status = %d, want 400", w.Code)
	}
	store.err = errTelemetryTimeout
	w = httptest.NewRecorder()
	h.Audit(w, req)
	if w.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout status = %d, want 504", w.Code)
	}
}

func TestTelemetryClickHouseSQLUsesTenantLimitsAndAliases(t *testing.T) {
	t.Parallel()

	q := telemetryQuery{
		TenantID: adminTestTenantA, Endpoint: telemetryEndpointRequest,
		From:  time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
		Limit: 10, Sort: telemetrySortDesc,
		Filters: map[string]string{string(RateLimitDimTargetHost): testExampleHost, telemetryFieldTag: "blue"},
	}
	sql := buildRequestListSQL(q)
	for _, want := range []string{
		"tenant_id = 'ten_a'",
		"max(timestamp) AS timestamp",
		"count() AS attempt_count",
		"has(tags, 'blue')",
		"GROUP BY request_id",
		") WHERE 1 = 1 ORDER BY timestamp DESC, request_id DESC",
		"LIMIT 11",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("sql missing %q: %s", want, sql)
		}
	}
}

func TestTelemetryAliasFiltersUsePublicRefSQL(t *testing.T) {
	t.Parallel()

	q := telemetryQuery{
		TenantID: adminTestTenantA, Endpoint: telemetryEndpointRequest,
		From:  time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
		Limit: 10, Sort: telemetrySortDesc,
		Filters: map[string]string{telemetryFieldAPIKeyRef: "key_public"},
	}
	sql := buildRequestListSQL(q)
	if !strings.Contains(sql, publicRefSQL("key_", "api_key_id")+" = 'key_public'") {
		t.Fatalf("api_key_ref SQL missing public alias filter: %s", sql)
	}
	auditQ := q
	auditQ.Endpoint = telemetryEndpointAudit
	auditQ.Filters = map[string]string{telemetryFieldActorRef: "key_actor"}
	sql = buildTelemetrySQL("config_audit_events", auditQ, auditFilterColumns, telemetryFieldResourceID, auditSelectColumns)
	if !strings.Contains(sql, publicRefSQL("key_", "actor_id")+" = 'key_actor'") {
		t.Fatalf("actor_ref SQL missing public alias filter: %s", sql)
	}
}

func TestTelemetryWorkersFiltersPublicRefAndOmitsTopology(t *testing.T) {
	t.Parallel()

	ref := stableWorkerRef(adminTestTenantA, telemetryTestWorker1)
	q := telemetryQuery{
		TenantID: adminTestTenantA, Endpoint: telemetryEndpointWorker,
		From:  time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC),
		To:    time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
		Limit: 10, Sort: telemetrySortDesc,
		Filters: map[string]string{telemetryFieldWorkerRef: ref},
	}
	sql := buildTelemetrySQL("worker_events", q, workerFilterColumns, telemetryFieldWorkerID, workerSelectColumns)
	if !strings.Contains(sql, workerRefSQL()+" = '"+ref+"'") {
		t.Fatalf("worker_ref SQL missing public alias filter: %s", sql)
	}
	cursor := nextCursor(q, q.From, telemetryTestWorker1)
	decoded, err := decodeTelemetryCursor(cursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if decoded.Filters[telemetryFieldWorkerRef] != ref {
		t.Fatalf("cursor worker_ref = %q, want %q", decoded.Filters[telemetryFieldWorkerRef], ref)
	}

	rows := []workerEventRow{
		{Timestamp: q.From, TenantID: adminTestTenantA, WorkerID: telemetryTestWorker1, EventType: "heartbeat"},
		{Timestamp: q.From, TenantID: adminTestTenantA, WorkerID: "worker_2", EventType: "heartbeat"},
	}
	items := make([]WorkerTelemetryItem, 0, len(rows))
	for _, row := range rows {
		item := row.item()
		if item.WorkerRef == q.Filters[telemetryFieldWorkerRef] {
			items = append(items, item)
		}
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal worker items: %v", err)
	}
	body := string(raw)
	if len(items) != 1 || items[0].WorkerRef != ref {
		t.Fatalf("worker_ref filter returned %#v, want only %q", items, ref)
	}
	for _, forbidden := range []string{telemetryFieldWorkerID, telemetryTestSessionID, telemetryTestSelectedExecutor} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("worker response leaked %s: %s", forbidden, body)
		}
	}
}

func TestTelemetryPaginationCursorOnlyWhenExtraRowExists(t *testing.T) {
	t.Parallel()

	q := telemetryQuery{TenantID: adminTestTenantA, Endpoint: telemetryEndpointRequest, Limit: 2, Sort: telemetrySortDesc}
	twoRows := []requestEventRow{{RequestID: requestMetadataTestRequestID}, {RequestID: requestMetadataTestNextRequestID}}
	trimmed, mark := trimPage(twoRows, q)
	if len(trimmed) != 2 || !mark.Timestamp.IsZero() {
		t.Fatalf("two-row trim = len %d mark %#v, want no cursor", len(trimmed), mark)
	}
	threeRows := []requestEventRow{{RequestID: requestMetadataTestRequestID}, {RequestID: requestMetadataTestNextRequestID, Timestamp: q.From}, {RequestID: "req_3"}}
	trimmed, mark = trimPage(threeRows, q)
	if len(trimmed) != 2 || mark.Tie != requestMetadataTestNextRequestID {
		t.Fatalf("three-row trim = len %d mark %#v, want cursor at second row", len(trimmed), mark)
	}
}

func TestTelemetryCursorBindsTimeWindow(t *testing.T) {
	t.Parallel()

	h, token, _ := newTestTelemetryHandlers(t, RoleViewer)
	from := time.Date(2026, 7, 6, 11, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	q := telemetryQuery{TenantID: adminTestTenantA, Endpoint: telemetryEndpointRequest, From: from, To: to, Limit: 2, Sort: telemetrySortDesc}
	cursor := nextCursor(q, from, requestMetadataTestRequestID)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/telemetry/requests?from=2026-07-06T10:00:00Z&to=2026-07-06T12:00:00Z&cursor="+cursor, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	h.Requests(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

type fakeTelemetryStore struct {
	last         telemetryQuery
	requests     []RequestTelemetryItem
	workers      []WorkerTelemetryItem
	detail       RequestTelemetryDetail
	detailTenant string
	err          error
}

func (s *fakeTelemetryStore) ListRequests(_ context.Context, q telemetryQuery) ([]RequestTelemetryItem, string, error) {
	s.last = q

	return s.requests, "", s.err
}

func (s *fakeTelemetryStore) RequestDetail(_ context.Context, tenantID, requestID string) (RequestTelemetryDetail, error) {
	s.detailTenant = tenantID
	if s.err != nil {
		return RequestTelemetryDetail{}, s.err
	}
	if s.detail.RequestID == "" || s.detail.RequestID != requestID {
		return RequestTelemetryDetail{}, errTelemetryNotFound
	}

	return s.detail, nil
}

func (s *fakeTelemetryStore) ListWorkers(_ context.Context, q telemetryQuery) ([]WorkerTelemetryItem, string, error) {
	s.last = q

	return s.workers, "", s.err
}

func (s *fakeTelemetryStore) ListAudit(_ context.Context, q telemetryQuery) ([]AuditTelemetryItem, string, error) {
	s.last = q

	return nil, "", s.err
}

func newTestTelemetryHandlers(t *testing.T, role Role) (*TelemetryHandlers, string, *fakeTelemetryStore) {
	t.Helper()
	apiKeys := NewInMemoryAPIKeyStore()
	gen, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey() error = %v", err)
	}
	mustCreate(t, apiKeys, APIKeyRecord{
		ID: "key_1", ScopeType: ScopeTenant, TenantID: adminTestTenantA, Role: role,
		Prefix: gen.Prefix, SecretHash: HashAPIKeySecret(gen.Secret, nil),
		Status: APIKeyStatusActive, CreatedAt: time.Now().UTC(),
	})
	store := &fakeTelemetryStore{}

	return &TelemetryHandlers{
		Authenticator: NewAuthenticator(apiKeys, nil),
		Store:         store,
		Now:           func() time.Time { return time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC) },
	}, gen.Secret, store
}

package control

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	telemetryDefaultLimit    = 100
	telemetryMaxLimit        = 500
	telemetryMaxCHReadRows   = 250000
	telemetryMaxCHReadBytes  = 64 << 20
	telemetryCHMaxExecSecs   = 5
	telemetryRequestWindow   = 24 * time.Hour
	telemetryWorkerWindow    = 24 * time.Hour
	telemetryAuditWindow     = 7 * 24 * time.Hour
	telemetryEndpointRequest = "requests"
	telemetryEndpointWorker  = "workers"
	telemetryEndpointAudit   = "audit"
	telemetrySortDesc        = "timestamp_desc"
	telemetrySortAsc         = "timestamp_asc"
	telemetryScannerMaxBytes = 4 << 20
	telemetryErrorMaxBytes   = 4096
	telemetryDetailWindow    = 24 * time.Hour

	telemetryFieldTimestamp       = "timestamp"
	telemetryFieldRequestID       = "request_id"
	telemetryFieldIngressType     = "ingress_type"
	telemetryFieldCountry         = "country"
	telemetryFieldClientStatus    = "client_status"
	telemetryFieldCaptureDecision = "capture_decision"
	telemetryFieldWorkerID        = "worker_id"
	telemetryFieldDraining        = "draining"
	telemetryFieldAction          = "action"
	telemetryFieldConfigVersion   = "config_version"
	telemetryFieldMethod          = "method"
	telemetryFieldExecutorType    = "executor_type"
	telemetryFieldErrorCategory   = "error_category"
	telemetryFieldTimeoutType     = "timeout_type"
	telemetryFieldEventType       = "event_type"
	telemetryFieldActorType       = "actor_type"
	telemetryFieldHealth          = "health"
	telemetryFieldConfigType      = "config_type"
	telemetryFieldTag             = "tag"
	telemetryFieldTraceID         = "trace_id"
	telemetryFieldPoolID          = "pool_id"
	telemetryFieldUpstreamStatus  = "upstream_status"
	telemetryFieldPath            = "field_path"
	telemetryFieldRegion          = "region"
	telemetryFieldResourceID      = "resource_id"
	telemetryFieldRouteID         = "route_id"
	telemetryFieldWorkerRef       = "worker_ref"
	telemetryFieldAPIKeyRef       = "api_" + "key_ref"
	telemetryFieldActorRef        = "actor_ref"
	telemetryFieldTags            = "tags"
)

var (
	errTelemetryNotFound  = errors.New("telemetry row not found")
	errTelemetryTimeout   = errors.New("clickhouse telemetry timeout")
	errTelemetryReadLimit = errors.New("clickhouse telemetry read limit")
)

// TelemetryStore reads tenant-scoped public telemetry rows.
type TelemetryStore interface {
	ListRequests(ctx context.Context, q telemetryQuery) ([]RequestTelemetryItem, string, error)
	RequestDetail(ctx context.Context, tenantID, requestID string) (RequestTelemetryDetail, error)
	ListWorkers(ctx context.Context, q telemetryQuery) ([]WorkerTelemetryItem, string, error)
	ListAudit(ctx context.Context, q telemetryQuery) ([]AuditTelemetryItem, string, error)
}

// TelemetryHandlers serves P1 tenant-facing telemetry read APIs.
type TelemetryHandlers struct {
	Authenticator *Authenticator
	Store         TelemetryStore
	Now           func() time.Time
}

type telemetryQuery struct {
	TenantID string
	Endpoint string
	From     time.Time
	To       time.Time
	Limit    int
	Sort     string
	Filters  map[string]string
	Cursor   telemetryCursor
}

type telemetryCursor struct {
	TenantID  string            `json:"tenant_id"`
	Endpoint  string            `json:"endpoint"`
	From      time.Time         `json:"from"`
	To        time.Time         `json:"to"`
	Filters   map[string]string `json:"filters"`
	Sort      string            `json:"sort"`
	Timestamp time.Time         `json:"timestamp"`
	Tie       string            `json:"tie"`
}

type telemetryEnvelope[T any] struct {
	Items      []T           `json:"items"`
	NextCursor string        `json:"next_cursor"`
	Query      telemetryEcho `json:"query"`
}

type telemetryEcho struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Limit int    `json:"limit"`
	Sort  string `json:"sort"`
}

// TelemetryTiming is the public per-phase duration shape.
type TelemetryTiming struct {
	RoutingMs    uint32 `json:"routing_ms"`
	AssignmentMs uint32 `json:"assignment_ms"`
	EgressMs     uint32 `json:"egress_ms"`
	TotalMs      uint32 `json:"total_ms"`
}

// RequestTelemetryItem is one public request telemetry summary.
type RequestTelemetryItem struct {
	Timestamp         string          `json:"timestamp"`
	RequestID         string          `json:"request_id"`
	TraceID           string          `json:"trace_id"`
	APIKeyRef         string          `json:"api_key_ref"`
	IngressType       string          `json:"ingress_type"`
	Method            string          `json:"method"`
	TargetHost        string          `json:"target_host"`
	TargetURL         string          `json:"target_url"`
	RouteID           string          `json:"route_id"`
	PoolID            string          `json:"pool_id"`
	ExecutorType      string          `json:"executor_type"`
	Country           string          `json:"country"`
	Region            string          `json:"region"`
	IPType            string          `json:"ip_type"`
	Tags              []string        `json:"tags"`
	AttemptCount      int             `json:"attempt_count"`
	UpstreamStatus    uint16          `json:"upstream_status"`
	ClientStatus      uint16          `json:"client_status"`
	ErrorCode         string          `json:"error_code"`
	ErrorCategory     string          `json:"error_category"`
	TimeoutType       string          `json:"timeout_type"`
	RequestSizeBytes  uint64          `json:"request_size_bytes"`
	ResponseSizeBytes uint64          `json:"response_size_bytes"`
	Timing            TelemetryTiming `json:"timing"`
	CaptureDecision   string          `json:"capture_decision"`
}

// RequestTelemetryDetail is the public request-attempt detail response.
type RequestTelemetryDetail struct {
	RequestTelemetryItem
	Attempts []RequestTelemetryAttempt `json:"attempts"`
}

// RequestTelemetryAttempt is one request attempt in detail responses.
type RequestTelemetryAttempt struct {
	Attempt        uint8           `json:"attempt"`
	Timestamp      string          `json:"timestamp"`
	ClientStatus   uint16          `json:"client_status"`
	UpstreamStatus uint16          `json:"upstream_status"`
	ErrorCode      string          `json:"error_code"`
	ErrorCategory  string          `json:"error_category"`
	TimeoutType    string          `json:"timeout_type"`
	Timing         TelemetryTiming `json:"timing"`
}

// WorkerTelemetryItem is one public worker health event.
type WorkerTelemetryItem struct {
	Timestamp         string `json:"timestamp"`
	WorkerRef         string `json:"worker_ref"`
	ExecutorType      string `json:"executor_type"`
	EventType         string `json:"event_type"`
	Health            string `json:"health"`
	ActiveRequests    uint32 `json:"active_requests"`
	MaxConcurrency    uint32 `json:"max_concurrency"`
	AvailableCapacity uint32 `json:"available_capacity"`
	Draining          bool   `json:"draining"`
	Reason            string `json:"reason"`
}

// AuditTelemetryItem is one public config audit event.
type AuditTelemetryItem struct {
	Timestamp     string `json:"timestamp"`
	ActorType     string `json:"actor_type"`
	ActorRef      string `json:"actor_ref"`
	ConfigType    string `json:"config_type"`
	ResourceID    string `json:"resource_id"`
	Action        string `json:"action"`
	ConfigVersion uint64 `json:"config_version"`
	FieldPath     string `json:"field_path"`
	OldValueJSON  string `json:"old_value_json"`
	NewValueJSON  string `json:"new_value_json"`
}

// Requests handles GET /api/v1/telemetry/requests.
func (h *TelemetryHandlers) Requests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	q, ok := h.parseQuery(w, r, telemetryEndpointRequest, telemetryRequestWindow, requestTelemetryFilters)
	if !ok {
		return
	}

	items, next, err := h.Store.ListRequests(r.Context(), q)
	writeTelemetryList(h, w, q, items, next, err)
}

// RequestDetail handles GET /api/v1/telemetry/requests/{request_id}.
func (h *TelemetryHandlers) RequestDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, ErrorResponseFromCode(InvalidRequest, "", nil))

		return
	}

	identity, ok := h.authorize(w, r, true)
	if !ok {
		return
	}

	requestID := strings.TrimPrefix(r.URL.Path, "/api/v1/telemetry/requests/")
	if requestID == "" || strings.Contains(requestID, "/") {
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: "request_id is required"}))

		return
	}

	tenantID, ok := h.resolveTenant(w, r, identity)
	if !ok {
		return
	}

	detail, err := h.Store.RequestDetail(r.Context(), tenantID, requestID)
	if errors.Is(err, errTelemetryNotFound) {
		WriteError(w, http.StatusNotFound, ErrorResponseFromCode(RouteNoMatch, "", nil))

		return
	}

	if err != nil {
		h.writeStoreError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// Workers handles GET /api/v1/telemetry/workers.
func (h *TelemetryHandlers) Workers(w http.ResponseWriter, r *http.Request) {
	q, ok := h.parseQuery(w, r, telemetryEndpointWorker, telemetryWorkerWindow, workerTelemetryFilters)
	if !ok {
		return
	}

	items, next, err := h.Store.ListWorkers(r.Context(), q)
	writeTelemetryList(h, w, q, items, next, err)
}

// Audit handles GET /api/v1/telemetry/audit.
func (h *TelemetryHandlers) Audit(w http.ResponseWriter, r *http.Request) {
	q, ok := h.parseQuery(w, r, telemetryEndpointAudit, telemetryAuditWindow, auditTelemetryFilters)
	if !ok {
		return
	}

	items, next, err := h.Store.ListAudit(r.Context(), q)
	writeTelemetryList(h, w, q, items, next, err)
}

func (h *TelemetryHandlers) parseQuery(w http.ResponseWriter, r *http.Request, endpoint string, maxWindow time.Duration, allowed map[string]bool) (telemetryQuery, bool) {
	identity, ok := h.authorize(w, r, true)
	if !ok {
		return telemetryQuery{}, false
	}

	tenantID, ok := h.resolveTenant(w, r, identity)
	if !ok {
		return telemetryQuery{}, false
	}

	qv := r.URL.Query()

	from, to, ok := h.parseWindow(w, qv, maxWindow)
	if !ok {
		return telemetryQuery{}, false
	}

	limit, sort, ok := h.parseLimitSort(w, qv)
	if !ok {
		return telemetryQuery{}, false
	}

	filters, ok := h.parseFilters(w, qv, allowed)
	if !ok {
		return telemetryQuery{}, false
	}

	q := telemetryQuery{TenantID: tenantID, Endpoint: endpoint, From: from, To: to, Limit: limit, Sort: sort, Filters: filters}

	cursor, ok := h.parseCursor(w, qv, q)
	if !ok {
		return telemetryQuery{}, false
	}

	if !cursor.Timestamp.IsZero() {
		q.Cursor = cursor
	}

	return q, true
}

func (h *TelemetryHandlers) parseWindow(w http.ResponseWriter, qv url.Values, maxWindow time.Duration) (time.Time, time.Time, bool) {
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now().UTC()
	}

	from := now.Add(-time.Hour)
	to := now

	if qv.Get("to") != "" {
		parsed, err := time.Parse(time.RFC3339Nano, qv.Get("to"))
		if err != nil {
			return time.Time{}, time.Time{}, h.invalid(w, "to must be RFC3339")
		}

		to = parsed.UTC()
	}

	if qv.Get("from") != "" {
		parsed, err := time.Parse(time.RFC3339Nano, qv.Get("from"))
		if err != nil {
			return time.Time{}, time.Time{}, h.invalid(w, "from must be RFC3339")
		}

		from = parsed.UTC()
	}

	if !from.Before(to) || to.Sub(from) > maxWindow {
		return time.Time{}, time.Time{}, h.invalid(w, "time window is invalid or too large")
	}

	return from, to, true
}

func (h *TelemetryHandlers) parseLimitSort(w http.ResponseWriter, qv url.Values) (int, string, bool) {
	limit := telemetryDefaultLimit

	if qv.Get("limit") != "" {
		parsed, err := strconv.Atoi(qv.Get("limit"))
		if err != nil || parsed < 1 || parsed > telemetryMaxLimit {
			return 0, "", h.invalid(w, "limit must be between 1 and 500")
		}

		limit = parsed
	}

	sort := qv.Get("sort")
	if sort == "" {
		sort = telemetrySortDesc
	}

	if sort != telemetrySortDesc && sort != telemetrySortAsc {
		return 0, "", h.invalid(w, "sort must be timestamp_desc or timestamp_asc")
	}

	return limit, sort, true
}

func (h *TelemetryHandlers) parseFilters(w http.ResponseWriter, qv url.Values, allowed map[string]bool) (map[string]string, bool) {
	filters := map[string]string{}

	for key, vals := range qv {
		if isTelemetryControlParam(key) {
			continue
		}

		if !allowed[key] || len(vals) != 1 {
			return nil, h.invalid(w, "unsupported telemetry filter")
		}

		val, ok := h.normalizeFilter(w, key, vals[0])
		if !ok {
			return nil, false
		}

		filters[key] = val
	}

	return filters, true
}

func (h *TelemetryHandlers) normalizeFilter(w http.ResponseWriter, key, val string) (string, bool) {
	if key == telemetryFieldDraining {
		switch strings.ToLower(val) {
		case "true", "1":
			return "1", true
		case "false", "0":
			return "0", true
		default:
			return "", h.invalid(w, "draining must be boolean")
		}
	}

	if isNumericTelemetryFilter(key) {
		parsed, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return "", h.invalid(w, "numeric telemetry filter is invalid")
		}

		return strconv.FormatUint(parsed, 10), true
	}

	return val, true
}

func (h *TelemetryHandlers) parseCursor(w http.ResponseWriter, qv url.Values, q telemetryQuery) (telemetryCursor, bool) {
	c := qv.Get("cursor")
	if c == "" {
		return telemetryCursor{}, true
	}

	cursor, err := decodeTelemetryCursor(c)
	if err != nil || cursor.TenantID != q.TenantID || cursor.Endpoint != q.Endpoint || cursor.Sort != q.Sort || !cursor.From.Equal(q.From) || !cursor.To.Equal(q.To) || !sameStringMap(cursor.Filters, q.Filters) {
		return telemetryCursor{}, h.invalid(w, "cursor does not match query")
	}

	return cursor, true
}

func isTelemetryControlParam(key string) bool {
	switch key {
	case "from", "to", "limit", "cursor", "sort", "tenant_id":
		return true
	default:
		return false
	}
}

func (h *TelemetryHandlers) authorize(w http.ResponseWriter, r *http.Request, allowRequester bool) (Identity, bool) {
	if h.Authenticator == nil {
		writeAuthOrRBACError(w, ErrAuthFailure)

		return Identity{}, false
	}

	identity, err := h.Authenticator.Authenticate(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		writeAuthOrRBACError(w, err)

		return Identity{}, false
	}

	allowed := []Role{RoleViewer, RoleOperator, RoleTenantAdmin, RoleSystemAdmin}
	if allowRequester {
		allowed = append(allowed, RoleRequester)
	}

	err = RequireRole(identity, allowed...)
	if err != nil {
		writeAuthOrRBACError(w, err)

		return Identity{}, false
	}

	return identity, true
}

func (h *TelemetryHandlers) resolveTenant(w http.ResponseWriter, r *http.Request, identity Identity) (string, bool) {
	if !identity.IsPlatform() {
		if r.URL.Query().Get("tenant_id") != "" {
			return "", h.invalid(w, "tenant_id cannot be overridden")
		}

		return identity.TenantID, true
	}

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		return "", h.invalid(w, "tenant_id is required for platform telemetry reads")
	}

	return tenantID, true
}

func writeTelemetryList[T any](h *TelemetryHandlers, w http.ResponseWriter, q telemetryQuery, items []T, next string, err error) {
	if err != nil {
		h.writeStoreError(w, err)

		return
	}

	writeJSON(w, http.StatusOK, telemetryEnvelope[T]{Items: items, NextCursor: next, Query: telemetryEcho{
		From: telemetryTime(q.From), To: telemetryTime(q.To), Limit: q.Limit, Sort: q.Sort,
	}})
}

func (h *TelemetryHandlers) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errTelemetryTimeout):
		WriteError(w, http.StatusGatewayTimeout, ErrorResponseFromCode(TimeoutExceeded, "", nil))
	case errors.Is(err, errTelemetryReadLimit):
		WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: "narrow the time window or filters"}))
	default:
		WriteError(w, http.StatusInternalServerError, ErrorResponseFromCode(ControlInternalError, "", nil))
	}
}

func (h *TelemetryHandlers) invalid(w http.ResponseWriter, reason string) bool {
	WriteError(w, http.StatusBadRequest, ErrorResponseFromCode(InvalidRequest, "", map[string]string{errorDetailReasonKey: reason}))

	return false
}

var requestTelemetryFilters = map[string]bool{
	telemetryFieldRequestID: true, telemetryFieldTraceID: true, telemetryFieldAPIKeyRef: true, telemetryFieldIngressType: true, telemetryFieldMethod: true, string(RateLimitDimTargetHost): true,
	telemetryFieldRouteID: true, telemetryFieldPoolID: true, telemetryFieldExecutorType: true, telemetryFieldCountry: true, telemetryFieldRegion: true, string(RateLimitDimIPType): true,
	telemetryFieldTag: true, telemetryFieldUpstreamStatus: true, telemetryFieldClientStatus: true, metricLabelErrorCode: true, telemetryFieldErrorCategory: true,
	telemetryFieldTimeoutType: true, telemetryFieldCaptureDecision: true,
}

var workerTelemetryFilters = map[string]bool{
	telemetryFieldWorkerRef: true, telemetryFieldExecutorType: true, telemetryFieldEventType: true, telemetryFieldHealth: true, telemetryFieldDraining: true,
}

var auditTelemetryFilters = map[string]bool{
	telemetryFieldActorType: true, telemetryFieldActorRef: true, telemetryFieldConfigType: true, telemetryFieldResourceID: true, telemetryFieldAction: true, telemetryFieldPath: true, telemetryFieldConfigVersion: true,
}

// HTTPClickHouseTelemetryStore reads telemetry rows through ClickHouse HTTP.
type HTTPClickHouseTelemetryStore struct {
	sink *HTTPClickHouseSink
}

// NewHTTPClickHouseTelemetryStore creates a ClickHouse-backed telemetry store.
func NewHTTPClickHouseTelemetryStore(sink *HTTPClickHouseSink) *HTTPClickHouseTelemetryStore {
	return &HTTPClickHouseTelemetryStore{sink: sink}
}

// ListRequests returns request telemetry summaries for one tenant.
func (s *HTTPClickHouseTelemetryStore) ListRequests(ctx context.Context, q telemetryQuery) ([]RequestTelemetryItem, string, error) {
	rows, next, err := queryClickHouse[requestEventRow](ctx, s.sink, buildRequestListSQL(q))
	if err != nil {
		return nil, "", err
	}

	rows, next = trimPage(rows, q)

	items := make([]RequestTelemetryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.requestItem())
	}

	return items, nextCursor(q, next.Timestamp, next.Tie), nil
}

// RequestDetail returns all attempts for one tenant-scoped request ID.
func (s *HTTPClickHouseTelemetryStore) RequestDetail(ctx context.Context, tenantID, requestID string) (RequestTelemetryDetail, error) {
	q := telemetryQuery{TenantID: tenantID, Endpoint: telemetryEndpointRequest, From: time.Unix(0, 0).UTC(), To: time.Now().UTC().Add(telemetryDetailWindow), Limit: telemetryMaxLimit, Sort: telemetrySortAsc, Filters: map[string]string{telemetryFieldRequestID: requestID}}

	rows, _, err := queryClickHouse[requestEventRow](ctx, s.sink, buildRequestDetailSQL(q))
	if err != nil {
		return RequestTelemetryDetail{}, err
	}

	if len(rows) == 0 {
		return RequestTelemetryDetail{}, errTelemetryNotFound
	}

	out := RequestTelemetryDetail{RequestTelemetryItem: rows[len(rows)-1].requestItem()}

	out.RequestID = requestID
	for _, row := range rows {
		out.Attempts = append(out.Attempts, row.requestAttempt())
	}

	return out, nil
}

// ListWorkers returns public worker telemetry for one tenant.
func (s *HTTPClickHouseTelemetryStore) ListWorkers(ctx context.Context, q telemetryQuery) ([]WorkerTelemetryItem, string, error) {
	rows, next, err := queryClickHouse[workerEventRow](ctx, s.sink, buildTelemetrySQL("worker_events", q, workerFilterColumns, telemetryFieldWorkerID, workerSelectColumns))
	if err != nil {
		return nil, "", err
	}

	rows, next = trimPage(rows, q)

	items := make([]WorkerTelemetryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.item())
	}

	return items, nextCursor(q, next.Timestamp, next.Tie), nil
}

// ListAudit returns public config audit telemetry for one tenant.
func (s *HTTPClickHouseTelemetryStore) ListAudit(ctx context.Context, q telemetryQuery) ([]AuditTelemetryItem, string, error) {
	rows, next, err := queryClickHouse[auditEventRow](ctx, s.sink, buildTelemetrySQL("config_audit_events", q, auditFilterColumns, telemetryFieldResourceID, auditSelectColumns))
	if err != nil {
		return nil, "", err
	}

	rows, next = trimPage(rows, q)

	items := make([]AuditTelemetryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.item())
	}

	return items, nextCursor(q, next.Timestamp, next.Tie), nil
}

type pageMark struct {
	Timestamp time.Time
	Tie       string
}

func queryClickHouse[T interface{ pageMark() pageMark }](ctx context.Context, sink *HTTPClickHouseSink, sql string) ([]T, pageMark, error) {
	if sink == nil {
		return nil, pageMark{}, errTelemetryReadLimit
	}

	body, err := sink.query(ctx, sql)
	if err != nil {
		return nil, pageMark{}, err
	}

	defer func() { _ = body.Close() }()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(nil, telemetryScannerMaxBytes)

	var rows []T

	for scanner.Scan() {
		var row T

		err := json.Unmarshal(scanner.Bytes(), &row)
		if err != nil {
			return nil, pageMark{}, fmt.Errorf("decode clickhouse telemetry: %w", err)
		}

		rows = append(rows, row)
	}

	err = scanner.Err()
	if err != nil {
		return nil, pageMark{}, fmt.Errorf("read clickhouse telemetry: %w", err)
	}

	var mark pageMark
	if len(rows) > 0 {
		mark = rows[len(rows)-1].pageMark()
	}

	return rows, mark, nil
}

func (s *HTTPClickHouseSink) query(ctx context.Context, sql string) (io.ReadCloser, error) {
	endpoint, err := url.Parse(s.endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse clickhouse endpoint: %w", err)
	}

	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, errTelemetryReadLimit
	}

	req := &http.Request{
		Method: http.MethodPost,
		URL:    endpoint,
		Body:   io.NopCloser(strings.NewReader(sql)),
		Header: make(http.Header),
	}
	req = req.WithContext(ctx)

	query := req.URL.Query()
	query.Set("database", s.database)
	query.Set("default_format", "JSONEachRow")
	query.Set("max_execution_time", strconv.Itoa(telemetryCHMaxExecSecs))
	query.Set("max_rows_to_read", strconv.Itoa(telemetryMaxCHReadRows))
	query.Set("max_bytes_to_read", strconv.Itoa(telemetryMaxCHReadBytes))
	req.URL.RawQuery = query.Encode()
	req.Header.Set(headerCanonicalContentType, "text/plain")

	if s.user != "" {
		req.SetBasicAuth(s.user, s.pass)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query clickhouse telemetry: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, clickHouseQueryStatusError(resp)
	}

	return resp.Body, nil
}

func clickHouseQueryStatusError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, telemetryErrorMaxBytes))

	msg := strings.ToLower(string(b))
	if resp.StatusCode == http.StatusGatewayTimeout || strings.Contains(msg, "timeout") || strings.Contains(msg, "time limit") {
		return errTelemetryTimeout
	}

	if strings.Contains(msg, "limit") || strings.Contains(msg, "too many") {
		return errTelemetryReadLimit
	}

	return fmt.Errorf("%w: %s", errClickHouseInsertFailed, resp.Status)
}

func buildTelemetrySQL(table string, q telemetryQuery, filterCols map[string]string, tieColumn string, columns []string) string {
	var b strings.Builder
	b.WriteString(buildTelemetryBaseSQL(table, q, filterCols, columns))
	appendTelemetryCursor(&b, q, tieColumn)
	appendTelemetryOrderLimit(&b, q, tieColumn)

	return b.String()
}

func buildTelemetryBaseSQL(table string, q telemetryQuery, filterCols map[string]string, columns []string) string {
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(strings.Join(columns, ", "))
	b.WriteString(" FROM ")
	b.WriteString(table)
	b.WriteString(" WHERE tenant_id = ")
	b.WriteString(chQuote(q.TenantID))
	b.WriteString(" AND timestamp >= ")
	b.WriteString(chTime(q.From))
	b.WriteString(" AND timestamp < ")
	b.WriteString(chTime(q.To))

	appendTelemetryFilters(&b, q.Filters, filterCols)

	return b.String()
}

func appendTelemetryOrderLimit(b *strings.Builder, q telemetryQuery, tieColumn string) {
	direction := "DESC"
	if q.Sort == telemetrySortAsc {
		direction = "ASC"
	}

	b.WriteString(" ORDER BY ")
	b.WriteString(telemetryFieldTimestamp)
	b.WriteString(" ")
	b.WriteString(direction)
	b.WriteString(", ")
	b.WriteString(tieColumn)
	b.WriteString(" ")
	b.WriteString(direction)
	b.WriteString(" LIMIT ")
	b.WriteString(strconv.Itoa(q.Limit + 1))
}

func appendTelemetryFilters(b *strings.Builder, filters map[string]string, filterCols map[string]string) {
	for key, val := range filters {
		if key == telemetryFieldTag {
			b.WriteString(" AND has(tags, ")
			b.WriteString(chQuote(val))
			b.WriteString(")")

			continue
		}

		col, val := telemetryFilterColumnValue(key, filterCols[key], val)
		if key == telemetryFieldWorkerRef || key == telemetryFieldAPIKeyRef || key == telemetryFieldActorRef {
			b.WriteString(" AND ")
			b.WriteString(col)
			b.WriteString(" = ")
			b.WriteString(chQuote(val))

			continue
		}

		b.WriteString(" AND ")
		b.WriteString(col)
		b.WriteString(" = ")

		if isNumericTelemetryFilter(key) {
			b.WriteString(val)
		} else {
			b.WriteString(chQuote(val))
		}
	}
}

func telemetryFilterColumnValue(key, col, val string) (string, string) {
	switch key {
	case telemetryFieldAPIKeyRef:
		return publicRefSQL("key_", "api_key_id"), val
	case telemetryFieldActorRef:
		return publicRefSQL("key_", "actor_id"), val
	case telemetryFieldWorkerRef:
		return workerRefSQL(), val
	default:
		return col, val
	}
}

func workerRefSQL() string {
	return "concat('wrkpub_', lower(substring(hex(SHA256(concat(tenant_id, worker_id))), 1, 16)))"
}

func publicRefSQL(prefix, column string) string {
	return "concat(" + chQuote(prefix) + ", lower(substring(hex(SHA256(" + column + ")), 1, 16)))"
}

func isNumericTelemetryFilter(key string) bool {
	switch key {
	case telemetryFieldUpstreamStatus, telemetryFieldClientStatus, telemetryFieldConfigVersion, telemetryFieldDraining:
		return true
	default:
		return false
	}
}

func appendTelemetryCursor(b *strings.Builder, q telemetryQuery, tieColumn string) {
	if q.Cursor.Timestamp.IsZero() {
		return
	}

	op := "<"
	if q.Sort == telemetrySortAsc {
		op = ">"
	}

	b.WriteString(" AND (timestamp ")
	b.WriteString(op)
	b.WriteString(" ")
	b.WriteString(chTime(q.Cursor.Timestamp))
	b.WriteString(" OR (timestamp = ")
	b.WriteString(chTime(q.Cursor.Timestamp))
	b.WriteString(" AND ")
	b.WriteString(tieColumn)
	b.WriteString(" ")
	b.WriteString(op)
	b.WriteString(" ")
	b.WriteString(chQuote(q.Cursor.Tie))
	b.WriteString("))")
}

func buildRequestListSQL(q telemetryQuery) string {
	inner := telemetryQuery{TenantID: q.TenantID, From: q.From, To: q.To, Filters: q.Filters}

	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString(strings.Join(requestSelectAliases, ", "))
	b.WriteString(" FROM (")
	b.WriteString(buildTelemetryBaseSQL("request_events", inner, requestFilterColumns, requestListColumns))
	b.WriteString(" GROUP BY request_id) WHERE 1 = 1")
	appendTelemetryCursor(&b, q, telemetryFieldRequestID)
	appendTelemetryOrderLimit(&b, q, telemetryFieldRequestID)

	return b.String()
}

func buildRequestDetailSQL(q telemetryQuery) string {
	sql := buildTelemetrySQL("request_events", q, requestFilterColumns, "attempt", requestSelectColumns)
	sql = strings.Replace(sql, "ORDER BY timestamp ASC, attempt ASC", "ORDER BY attempt ASC, timestamp ASC", 1)

	return sql
}

func trimPage[T interface{ pageMark() pageMark }](rows []T, q telemetryQuery) ([]T, pageMark) {
	if len(rows) <= q.Limit {
		return rows, pageMark{}
	}

	rows = rows[:q.Limit]

	return rows, rows[len(rows)-1].pageMark()
}

func nextCursor(q telemetryQuery, ts time.Time, tie string) string {
	if ts.IsZero() || tie == "" {
		return ""
	}

	return encodeTelemetryCursor(telemetryCursor{TenantID: q.TenantID, Endpoint: q.Endpoint, From: q.From, To: q.To, Filters: q.Filters, Sort: q.Sort, Timestamp: ts, Tie: tie})
}

func encodeTelemetryCursor(c telemetryCursor) string {
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}

	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeTelemetryCursor(s string) (telemetryCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return telemetryCursor{}, fmt.Errorf("decode telemetry cursor: %w", err)
	}

	var c telemetryCursor

	err = json.Unmarshal(raw, &c)
	if err != nil {
		return telemetryCursor{}, fmt.Errorf("unmarshal telemetry cursor: %w", err)
	}

	return c, nil
}

func sameStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}

	for k, v := range a {
		if b[k] != v {
			return false
		}
	}

	return true
}

func chQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')

	for _, r := range s {
		if r == '\\' || r == '\'' {
			b.WriteByte('\\')
		}

		b.WriteRune(r)
	}

	b.WriteByte('\'')

	return b.String()
}

func chTime(t time.Time) string {
	return "parseDateTime64BestEffort(" + chQuote(t.UTC().Format(time.RFC3339Nano)) + ")"
}

func telemetryTime(t time.Time) string {
	return t.UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
}

func publicRef(prefix, raw string) string {
	if raw == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(raw))

	return prefix + hex.EncodeToString(sum[:8])
}

func sanitizeTelemetryURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""

	return u.String()
}

func boolUint(v uint8) bool { return v != 0 }

func stableWorkerRef(tenantID, workerID string) string {
	sum := sha256.Sum256([]byte(tenantID + workerID))

	return "wrkpub_" + hex.EncodeToString(sum[:8])
}

var (
	requestSelectColumns = []string{telemetryFieldTimestamp, telemetryFieldRequestID, telemetryFieldTraceID, "api_key_id", telemetryFieldIngressType, telemetryFieldMethod, string(RateLimitDimTargetHost), "target_url", telemetryFieldRouteID, telemetryFieldPoolID, telemetryFieldExecutorType, telemetryFieldCountry, telemetryFieldRegion, string(RateLimitDimIPType), telemetryFieldTags, "attempt", telemetryFieldUpstreamStatus, telemetryFieldClientStatus, metricLabelErrorCode, telemetryFieldErrorCategory, telemetryFieldTimeoutType, "request_size_bytes", "response_size_bytes", "routing_ms", "assignment_ms", "egress_ms", "total_ms", telemetryFieldCaptureDecision}
	requestListColumns   = []string{
		"max(timestamp) AS request_timestamp", telemetryFieldRequestID, "argMax(trace_id, timestamp) AS trace_id", "argMax(api_key_id, timestamp) AS api_key_id",
		"argMax(ingress_type, timestamp) AS ingress_type", "argMax(method, timestamp) AS method", "argMax(target_host, timestamp) AS target_host",
		"argMax(target_url, timestamp) AS target_url", "argMax(route_id, timestamp) AS route_id", "argMax(pool_id, timestamp) AS pool_id",
		"argMax(executor_type, timestamp) AS executor_type", "argMax(country, timestamp) AS country", "argMax(region, timestamp) AS region",
		"argMax(ip_type, timestamp) AS ip_type", "argMax(tags, timestamp) AS tags", "count() AS attempt_count",
		"argMax(upstream_status, timestamp) AS upstream_status", "argMax(client_status, timestamp) AS client_status",
		"argMax(error_code, timestamp) AS error_code", "argMax(error_category, timestamp) AS error_category", "argMax(timeout_type, timestamp) AS timeout_type",
		"argMax(request_size_bytes, timestamp) AS request_size_bytes", "argMax(response_size_bytes, timestamp) AS response_size_bytes",
		"argMax(routing_ms, timestamp) AS routing_ms", "argMax(assignment_ms, timestamp) AS assignment_ms", "argMax(egress_ms, timestamp) AS egress_ms",
		"argMax(total_ms, timestamp) AS total_ms", "argMax(capture_decision, timestamp) AS capture_decision",
	}
	requestSelectAliases = []string{
		"request_timestamp AS " + telemetryFieldTimestamp, telemetryFieldRequestID, telemetryFieldTraceID, "api_key_id", telemetryFieldIngressType, telemetryFieldMethod,
		string(RateLimitDimTargetHost), "target_url", telemetryFieldRouteID, telemetryFieldPoolID, telemetryFieldExecutorType, telemetryFieldCountry,
		telemetryFieldRegion, string(RateLimitDimIPType), telemetryFieldTags, "attempt_count", telemetryFieldUpstreamStatus, telemetryFieldClientStatus,
		metricLabelErrorCode, telemetryFieldErrorCategory, telemetryFieldTimeoutType, "request_size_bytes", "response_size_bytes", "routing_ms",
		"assignment_ms", "egress_ms", "total_ms", telemetryFieldCaptureDecision,
	}
)

var (
	workerSelectColumns = []string{telemetryFieldTimestamp, "tenant_id", telemetryFieldWorkerID, telemetryFieldExecutorType, telemetryFieldEventType, telemetryFieldHealth, "active_requests", "max_concurrency", "available_capacity", telemetryFieldDraining, "reason"}
	auditSelectColumns  = []string{telemetryFieldTimestamp, telemetryFieldActorType, "actor_id", telemetryFieldConfigType, telemetryFieldResourceID, telemetryFieldAction, telemetryFieldConfigVersion, telemetryFieldPath, "old_value_json", "new_value_json"}
)

var (
	requestFilterColumns = map[string]string{telemetryFieldRequestID: telemetryFieldRequestID, telemetryFieldTraceID: telemetryFieldTraceID, telemetryFieldIngressType: telemetryFieldIngressType, telemetryFieldMethod: telemetryFieldMethod, string(RateLimitDimTargetHost): string(RateLimitDimTargetHost), telemetryFieldRouteID: telemetryFieldRouteID, telemetryFieldPoolID: telemetryFieldPoolID, telemetryFieldExecutorType: telemetryFieldExecutorType, telemetryFieldCountry: telemetryFieldCountry, telemetryFieldRegion: telemetryFieldRegion, string(RateLimitDimIPType): string(RateLimitDimIPType), telemetryFieldUpstreamStatus: telemetryFieldUpstreamStatus, telemetryFieldClientStatus: telemetryFieldClientStatus, metricLabelErrorCode: metricLabelErrorCode, telemetryFieldErrorCategory: telemetryFieldErrorCategory, telemetryFieldTimeoutType: telemetryFieldTimeoutType, telemetryFieldCaptureDecision: telemetryFieldCaptureDecision}
	workerFilterColumns  = map[string]string{telemetryFieldWorkerRef: telemetryFieldWorkerRef, telemetryFieldExecutorType: telemetryFieldExecutorType, telemetryFieldEventType: telemetryFieldEventType, telemetryFieldHealth: telemetryFieldHealth, telemetryFieldDraining: telemetryFieldDraining}
	auditFilterColumns   = map[string]string{telemetryFieldActorType: telemetryFieldActorType, telemetryFieldConfigType: telemetryFieldConfigType, telemetryFieldResourceID: telemetryFieldResourceID, telemetryFieldAction: telemetryFieldAction, telemetryFieldPath: telemetryFieldPath, telemetryFieldConfigVersion: telemetryFieldConfigVersion}
)

type requestEventRow struct {
	Timestamp         time.Time `json:"timestamp"`
	RequestID         string    `json:"request_id"`
	TraceID           string    `json:"trace_id"`
	APIKeyID          string    `json:"api_key_id"`
	IngressType       string    `json:"ingress_type"`
	Method            string    `json:"method"`
	TargetHost        string    `json:"target_host"`
	TargetURL         string    `json:"target_url"`
	RouteID           string    `json:"route_id"`
	PoolID            string    `json:"pool_id"`
	ExecutorType      string    `json:"executor_type"`
	Country           string    `json:"country"`
	Region            string    `json:"region"`
	IPType            string    `json:"ip_type"`
	Tags              []string  `json:"tags"`
	Attempt           uint8     `json:"attempt"`
	AttemptCount      int       `json:"attempt_count"`
	UpstreamStatus    uint16    `json:"upstream_status"`
	ClientStatus      uint16    `json:"client_status"`
	ErrorCode         string    `json:"error_code"`
	ErrorCategory     string    `json:"error_category"`
	TimeoutType       string    `json:"timeout_type"`
	RequestSizeBytes  uint64    `json:"request_size_bytes"`
	ResponseSizeBytes uint64    `json:"response_size_bytes"`
	RoutingMs         uint32    `json:"routing_ms"`
	AssignmentMs      uint32    `json:"assignment_ms"`
	EgressMs          uint32    `json:"egress_ms"`
	TotalMs           uint32    `json:"total_ms"`
	CaptureDecision   string    `json:"capture_decision"`
}

func (r requestEventRow) pageMark() pageMark {
	return pageMark{Timestamp: r.Timestamp, Tie: r.RequestID}
}

func (r requestEventRow) timing() TelemetryTiming {
	return TelemetryTiming{RoutingMs: r.RoutingMs, AssignmentMs: r.AssignmentMs, EgressMs: r.EgressMs, TotalMs: r.TotalMs}
}

func (r requestEventRow) requestItem() RequestTelemetryItem {
	count := r.AttemptCount
	if count == 0 {
		count = 1
	}

	return RequestTelemetryItem{Timestamp: telemetryTime(r.Timestamp), RequestID: r.RequestID, TraceID: r.TraceID, APIKeyRef: publicRef("key_", r.APIKeyID), IngressType: r.IngressType, Method: r.Method, TargetHost: r.TargetHost, TargetURL: sanitizeTelemetryURL(r.TargetURL), RouteID: r.RouteID, PoolID: r.PoolID, ExecutorType: r.ExecutorType, Country: r.Country, Region: r.Region, IPType: r.IPType, Tags: r.Tags, AttemptCount: count, UpstreamStatus: r.UpstreamStatus, ClientStatus: r.ClientStatus, ErrorCode: r.ErrorCode, ErrorCategory: r.ErrorCategory, TimeoutType: r.TimeoutType, RequestSizeBytes: r.RequestSizeBytes, ResponseSizeBytes: r.ResponseSizeBytes, Timing: r.timing(), CaptureDecision: r.CaptureDecision}
}

func (r requestEventRow) requestAttempt() RequestTelemetryAttempt {
	return RequestTelemetryAttempt{Attempt: r.Attempt, Timestamp: telemetryTime(r.Timestamp), ClientStatus: r.ClientStatus, UpstreamStatus: r.UpstreamStatus, ErrorCode: r.ErrorCode, ErrorCategory: r.ErrorCategory, TimeoutType: r.TimeoutType, Timing: r.timing()}
}

type workerEventRow struct {
	Timestamp         time.Time `json:"timestamp"`
	TenantID          string    `json:"tenant_id"`
	WorkerID          string    `json:"worker_id"`
	ExecutorType      string    `json:"executor_type"`
	EventType         string    `json:"event_type"`
	Health            string    `json:"health"`
	ActiveRequests    uint32    `json:"active_requests"`
	MaxConcurrency    uint32    `json:"max_concurrency"`
	AvailableCapacity uint32    `json:"available_capacity"`
	Draining          uint8     `json:"draining"`
	Reason            string    `json:"reason"`
}

func (r workerEventRow) pageMark() pageMark { return pageMark{Timestamp: r.Timestamp, Tie: r.WorkerID} }
func (r workerEventRow) item() WorkerTelemetryItem {
	return WorkerTelemetryItem{Timestamp: telemetryTime(r.Timestamp), WorkerRef: stableWorkerRef(r.TenantID, r.WorkerID), ExecutorType: r.ExecutorType, EventType: r.EventType, Health: r.Health, ActiveRequests: r.ActiveRequests, MaxConcurrency: r.MaxConcurrency, AvailableCapacity: r.AvailableCapacity, Draining: boolUint(r.Draining), Reason: r.Reason}
}

type auditEventRow struct {
	Timestamp     time.Time `json:"timestamp"`
	ActorType     string    `json:"actor_type"`
	ActorID       string    `json:"actor_id"`
	ConfigType    string    `json:"config_type"`
	ResourceID    string    `json:"resource_id"`
	Action        string    `json:"action"`
	ConfigVersion uint64    `json:"config_version"`
	FieldPath     string    `json:"field_path"`
	OldValueJSON  string    `json:"old_value_json"`
	NewValueJSON  string    `json:"new_value_json"`
}

func (r auditEventRow) pageMark() pageMark {
	return pageMark{Timestamp: r.Timestamp, Tie: r.ResourceID}
}

func (r auditEventRow) item() AuditTelemetryItem {
	return AuditTelemetryItem{Timestamp: telemetryTime(r.Timestamp), ActorType: r.ActorType, ActorRef: publicRef("key_", r.ActorID), ConfigType: r.ConfigType, ResourceID: r.ResourceID, Action: r.Action, ConfigVersion: r.ConfigVersion, FieldPath: r.FieldPath, OldValueJSON: r.OldValueJSON, NewValueJSON: r.NewValueJSON}
}

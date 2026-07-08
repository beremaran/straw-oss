package control

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
	"github.com/beremaran/straw/v2/internal/objectstore"
)

const (
	captureTestReqKey  = "req-key"
	captureTestRespKey = "resp-key"
)

// fakeCaptureBodyStore records uploads/deletes and lets a test force one
// direction to fail, so partial-failure cleanup and outage paths are testable
// without a live object store.
type fakeCaptureBodyStore struct {
	uploads  []string // "request:<tenant>/<request>" style records
	deletes  []string
	failReq  error
	failResp error
	keyReq   string
	keyResp  string
}

func (f *fakeCaptureBodyStore) UploadRequestBody(_ context.Context, tenantID, requestID string, _ []byte) (*strawpb.BodyRefFrame, error) {
	if f.failReq != nil {
		return nil, f.failReq
	}

	f.uploads = append(f.uploads, "request:"+tenantID+"/"+requestID)
	key := f.keyReq
	if key == "" {
		key = "tenant/" + tenantID + "/request/" + requestID + "/request/nonce"
	}

	return frameWithKey(key), nil
}

func (f *fakeCaptureBodyStore) UploadResponseBody(_ context.Context, tenantID, requestID string, _ []byte) (*strawpb.BodyRefFrame, error) {
	if f.failResp != nil {
		return nil, f.failResp
	}

	f.uploads = append(f.uploads, "response:"+tenantID+"/"+requestID)
	key := f.keyResp
	if key == "" {
		key = "tenant/" + tenantID + "/request/" + requestID + "/response/nonce"
	}

	return frameWithKey(key), nil
}

func (f *fakeCaptureBodyStore) DeleteRequestBody(_ context.Context, frame *strawpb.BodyRefFrame) error {
	f.deletes = append(f.deletes, frame.GetS3().GetObjectKey())

	return nil
}

func (f *fakeCaptureBodyStore) DeleteResponseBody(_ context.Context, frame *strawpb.BodyRefFrame) error {
	f.deletes = append(f.deletes, frame.GetS3().GetObjectKey())

	return nil
}

func frameWithKey(key string) *strawpb.BodyRefFrame {
	return &strawpb.BodyRefFrame{Ref: &strawpb.BodyRefFrame_S3{S3: &strawpb.S3BodyRef{ObjectKey: key}}}
}

type recordingCaptureRecorder struct {
	events []PayloadCaptureEvent
}

func (r *recordingCaptureRecorder) Enqueue(event PayloadCaptureEvent) {
	r.events = append(r.events, event)
}

func headerPair(name, value string) HeaderPair {
	return HeaderPair{Name: name, Value: value}
}

func TestPayloadCaptureStoreBodiesByReference(t *testing.T) {
	bodies := &fakeCaptureBodyStore{keyReq: captureTestReqKey, keyResp: captureTestRespKey}
	rec := &recordingCaptureRecorder{}
	store := NewPayloadCaptureStore(bodies, rec)

	result := CaptureResult{
		CapturedAt:      time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC),
		Decision:        CaptureDecisionBodyFull,
		RequestHeaders:  "[]",
		ResponseHeaders: "[]",
		RequestBody:     []byte("request-body"),
		ResponseBody:    []byte("response-body"),
		Truncated:       true,
	}

	err := store.Store(context.Background(), PayloadCaptureMeta{TenantID: "t1", RequestID: "r1", CaptureScope: string(ScopeTenant)}, result)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}

	ev := rec.events[0]
	if ev.RequestBodyRef != captureTestReqKey || ev.ResponseBodyRef != captureTestRespKey {
		t.Fatalf("body refs = %q/%q, want %s/%s", ev.RequestBodyRef, ev.ResponseBodyRef, captureTestReqKey, captureTestRespKey)
	}

	if ev.TenantID != "t1" || ev.RequestID != "r1" || ev.CaptureScope != string(ScopeTenant) {
		t.Fatalf("identity fields = %+v", ev)
	}

	if ev.CaptureDecision != string(CaptureDecisionBodyFull) || ev.Truncated != 1 {
		t.Fatalf("decision/truncated = %q/%d", ev.CaptureDecision, ev.Truncated)
	}

	if len(bodies.uploads) != 2 {
		t.Fatalf("uploads = %v, want request+response", bodies.uploads)
	}
}

func TestPayloadCaptureStoreNoBodyNoUpload(t *testing.T) {
	bodies := &fakeCaptureBodyStore{}
	rec := &recordingCaptureRecorder{}
	store := NewPayloadCaptureStore(bodies, rec)

	// METADATA_ONLY-style result: headers empty, no bodies.
	result := CaptureResult{Decision: CaptureDecisionMetadataOnly}

	err := store.Store(context.Background(), PayloadCaptureMeta{TenantID: "t1", RequestID: "r1"}, result)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	if len(bodies.uploads) != 0 {
		t.Fatalf("uploads = %v, want none", bodies.uploads)
	}

	if len(rec.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(rec.events))
	}

	ev := rec.events[0]
	if ev.RequestBodyRef != "" || ev.ResponseBodyRef != "" {
		t.Fatalf("body refs = %q/%q, want empty", ev.RequestBodyRef, ev.ResponseBodyRef)
	}

	if ev.RedactedFields == nil {
		t.Fatalf("RedactedFields is nil; must serialize as [] for ClickHouse Array(String)")
	}
}

func TestPayloadCaptureStoreResponseFailureCleansUpRequest(t *testing.T) {
	bodies := &fakeCaptureBodyStore{keyReq: captureTestReqKey, failResp: objectstore.ErrUnavailable}
	rec := &recordingCaptureRecorder{}
	store := NewPayloadCaptureStore(bodies, rec)

	result := CaptureResult{
		Decision:     CaptureDecisionBodyFull,
		RequestBody:  []byte("request-body"),
		ResponseBody: []byte("response-body"),
	}

	err := store.Store(context.Background(), PayloadCaptureMeta{TenantID: "t1", RequestID: "r1"}, result)
	if !objectstore.IsUnavailable(err) {
		t.Fatalf("Store() error = %v, want object-storage unavailable", err)
	}

	if len(rec.events) != 0 {
		t.Fatalf("recorded %d events, want 0 on failed upload", len(rec.events))
	}

	if len(bodies.deletes) != 1 || bodies.deletes[0] != "req-key" {
		t.Fatalf("deletes = %v, want [req-key] cleanup", bodies.deletes)
	}
}

func TestPayloadCaptureStoreRequestOutageNoRow(t *testing.T) {
	bodies := &fakeCaptureBodyStore{failReq: objectstore.ErrUnavailable}
	rec := &recordingCaptureRecorder{}
	store := NewPayloadCaptureStore(bodies, rec)

	result := CaptureResult{Decision: CaptureDecisionBodyFull, RequestBody: []byte("body")}

	err := store.Store(context.Background(), PayloadCaptureMeta{TenantID: "t1", RequestID: "r1"}, result)
	if !objectstore.IsUnavailable(err) {
		t.Fatalf("Store() error = %v, want unavailable", err)
	}

	if len(rec.events) != 0 {
		t.Fatalf("recorded a row despite object-storage outage: %v", rec.events)
	}

	if len(bodies.deletes) != 0 {
		t.Fatalf("deleted objects but nothing was uploaded: %v", bodies.deletes)
	}
}

func TestPayloadCaptureStoreMissingBodyStoreNoRow(t *testing.T) {
	rec := &recordingCaptureRecorder{}
	store := NewPayloadCaptureStore(nil, rec)

	err := store.Store(context.Background(), PayloadCaptureMeta{TenantID: "t1", RequestID: "r1"}, CaptureResult{
		Decision:    CaptureDecisionBodyFull,
		RequestBody: []byte("body"),
	})
	if !objectstore.IsUnavailable(err) {
		t.Fatalf("Store() error = %v, want unavailable", err)
	}

	if len(rec.events) != 0 {
		t.Fatalf("recorded a row despite missing body store: %v", rec.events)
	}
}

func TestPayloadCaptureStorePersistsRedactedHeaders(t *testing.T) {
	bodies := &fakeCaptureBodyStore{}
	rec := &recordingCaptureRecorder{}
	store := NewPayloadCaptureStore(bodies, rec)

	// Drive the real capture engine so the store persists whatever it redacts.
	result := CapturePayload(
		CaptureDecisionHeaders,
		[]HeaderPair{headerPair(testAuthorizationHeader, "secret-token"), headerPair("X-Trace", "keep")},
		nil,
		nil,
		nil,
		CaptureOptions{},
	)

	err := store.Store(context.Background(), PayloadCaptureMeta{TenantID: "t1", RequestID: "r1"}, result)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	ev := rec.events[0]
	if strings.Contains(ev.RequestHeaders, "secret-token") {
		t.Fatalf("stored headers leaked secret: %s", ev.RequestHeaders)
	}

	if !strings.Contains(ev.RequestHeaders, requestMetadataRedacted) {
		t.Fatalf("stored headers not redacted: %s", ev.RequestHeaders)
	}

	found := false
	for _, f := range ev.RedactedFields {
		if f == testAuthorizationHeader {
			found = true
		}
	}

	if !found {
		t.Fatalf("redacted_fields = %v, want %s listed", ev.RedactedFields, testAuthorizationHeader)
	}
}

func TestHTTPClickHouseSinkPayloadCaptureFormat(t *testing.T) {
	var gotQuery url.Values

	var gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := NewHTTPClickHouseSink(srv.URL, "straw", "", "", srv.Client())
	err := sink.WritePayloadCaptureEvents(context.Background(), []PayloadCaptureEvent{{
		CapturedAt:      time.Date(2026, 7, 7, 16, 44, 11, int(812*time.Millisecond), time.UTC),
		RequestID:       "r1",
		TenantID:        "t1",
		CaptureDecision: string(CaptureDecisionBodyFull),
		RequestBodyRef:  "req-key",
		RedactedFields:  []string{"authorization"},
	}})
	if err != nil {
		t.Fatalf("WritePayloadCaptureEvents() error = %v", err)
	}

	if got := gotQuery.Get("query"); got != "INSERT INTO payload_capture_events FORMAT JSONEachRow" {
		t.Fatalf("query = %q", got)
	}

	if got := gotQuery.Get("date_time_input_format"); got != "best_effort" {
		t.Fatalf("date_time_input_format = %q, want best_effort", got)
	}

	if !strings.Contains(gotBody, `"captured_at":"2026-07-07T16:44:11`) {
		t.Fatalf("captured_at not RFC3339: %s", gotBody)
	}

	if !strings.Contains(gotBody, `"request_body_ref":"req-key"`) || !strings.Contains(gotBody, `"redacted_fields":["authorization"]`) {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestHTTPClickHouseSinkPayloadCaptureNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sink := NewHTTPClickHouseSink(srv.URL, "straw", "", "", srv.Client())
	err := sink.WritePayloadCaptureEvents(context.Background(), []PayloadCaptureEvent{{RequestID: "r1"}})
	if err == nil {
		t.Fatal("WritePayloadCaptureEvents() error = nil, want non-2xx error")
	}
}

func TestPayloadCaptureSchemaRetentionAndRefs(t *testing.T) {
	b, err := os.ReadFile("../../../infra/clickhouse-schema.sql")
	if err != nil {
		t.Fatalf("read clickhouse schema: %v", err)
	}

	schema := string(b)
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS straw.payload_capture_events",
		"request_body_ref  String",
		"response_body_ref String",
		"redacted_fields   Array(String)",
		"TTL toDateTime(captured_at) + INTERVAL 7 DAY",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("payload_capture_events schema missing %q", want)
		}
	}
}

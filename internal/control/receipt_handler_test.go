package control

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/internal/objectstore"
	"github.com/beremaran/straw-oss/internal/receipt"
)

func TestReceiptHandlerLifecycleAndSignedObjectBoundary(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	service, err := receipt.New(objectstore.Local{Root: t.TempDir()}, receipt.Config{DownloadBaseURL: "http://control", SigningKey: []byte("01234567890123456789012345678901"), MaxObjectBytes: 1024, MaxPartBytes: 1024, Retention: time.Hour, AssignmentTTL: time.Minute, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewReceiptHandler(service, NewDeploymentAuthenticator("")).Register(mux)
	body := []byte("receipt body")
	sum := sha256.Sum256(body)
	create := performReceiptRequest(t, mux, http.MethodPost, "/api/v1/receipts", bytes.NewReader([]byte(`{"direction":"request","size_bytes":12,"sha256_hex":"`+hex.EncodeToString(sum[:])+`"}`)), -1)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		ReceiptID string `json:"receipt_id"`
	}
	_ = json.Unmarshal(create.Body.Bytes(), &created)
	upload := performReceiptRequest(t, mux, http.MethodPut, "/api/v1/receipts/"+created.ReceiptID+"/parts/1", bytes.NewReader(body), int64(len(body)))
	if upload.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", upload.Code, upload.Body.String())
	}
	complete := performReceiptRequest(t, mux, http.MethodPost, "/api/v1/receipts/"+created.ReceiptID+"/complete", nil, 0)
	if complete.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", complete.Code, complete.Body.String())
	}
	ref, err := service.PrepareRequest(context.Background(), "default", created.ReceiptID, "req_1")
	if err != nil {
		t.Fatal(err)
	}
	signed, _ := url.Parse(ref.GetS3().GetSignedUrl())
	download := performReceiptRequest(t, mux, http.MethodGet, signed.RequestURI(), nil, 0)
	if download.Code != http.StatusOK || download.Body.String() != string(body) {
		t.Fatalf("download=%d %q", download.Code, download.Body.String())
	}
	tampered := signed.Query()
	tampered.Set("request_id", "req_other")
	denied := performReceiptRequest(t, mux, http.MethodGet, signed.Path+"?"+tampered.Encode(), nil, 0)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("tampered status=%d", denied.Code)
	}
}

func performReceiptRequest(t *testing.T, handler http.Handler, method, target string, body io.Reader, size int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, target, body)
	if size >= 0 {
		req.ContentLength = size
		req.Header.Set("Content-Length", strconv.FormatInt(size, 10))
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	return rec
}

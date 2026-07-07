package control

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw/v2/internal/objectstore"
)

const (
	testBodyRefEnvA   = "STRAW_TEST_BODYREF_A"
	testBodyRefEnvB   = "STRAW_TEST_BODYREF_B"
	testBodyRefBucket = "bucket"
	testBodyRefRegion = "us-east-1"
)

func TestS3RequestBodyRefStoreUploadsScopedRequestBody(t *testing.T) {
	body := []byte("large request body")
	putBody := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/bucket/tenant/ten_a/request/req_1/request/") {
			t.Fatalf("path = %q, want tenant/request scoped object", r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		putBody <- string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	t.Setenv(testBodyRefEnvA, "id-value")
	t.Setenv(testBodyRefEnvB, "signing-value")
	client, err := objectstore.New(objectstore.Options{
		Enabled:       true,
		Endpoint:      server.URL,
		Bucket:        testBodyRefBucket,
		Region:        testBodyRefRegion,
		AccessKeyEnv:  testBodyRefEnvA,
		SecretKeyEnv:  testBodyRefEnvB,
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("objectstore.New() error = %v", err)
	}

	store := NewS3RequestBodyRefStore(client)
	store.Now = func() time.Time { return time.Unix(100, 0) }

	frame, err := store.UploadRequestBody(context.Background(), "ten_a", "req_1", body)
	if err != nil {
		t.Fatalf("UploadRequestBody() error = %v", err)
	}
	if got := <-putBody; got != string(body) {
		t.Fatalf("uploaded body = %q, want %q", got, string(body))
	}
	if frame.GetExpectedSizeBytes() != uint64(len(body)) || frame.GetSha256Hex() == "" {
		t.Fatalf("frame integrity fields = size %d sha %q", frame.GetExpectedSizeBytes(), frame.GetSha256Hex())
	}
	if ref := frame.GetS3(); ref == nil || !strings.HasPrefix(ref.GetObjectKey(), "tenant/ten_a/request/req_1/request/") || ref.GetSignedUrl() == "" || ref.GetExpiresUnixMs() <= 0 {
		t.Fatalf("S3 ref = %#v, want scoped object key and signed URL", ref)
	}
}

func TestS3RequestBodyRefStoreUploadsScopedResponseBody(t *testing.T) {
	body := []byte("large response body teed to object storage")
	putBody := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/bucket/tenant/ten_a/request/req_1/response/") {
			t.Fatalf("path = %q, want tenant/request/response scoped object", r.URL.Path)
		}
		if r.Header.Get("x-amz-server-side-encryption") == "" {
			t.Fatal("PUT missing server-side-encryption header")
		}
		raw, _ := io.ReadAll(r.Body)
		putBody <- string(raw)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	t.Setenv(testBodyRefEnvA, "id-value")
	t.Setenv(testBodyRefEnvB, "signing-value")
	client, err := objectstore.New(objectstore.Options{
		Enabled:       true,
		Endpoint:      server.URL,
		Bucket:        testBodyRefBucket,
		Region:        testBodyRefRegion,
		AccessKeyEnv:  testBodyRefEnvA,
		SecretKeyEnv:  testBodyRefEnvB,
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("objectstore.New() error = %v", err)
	}

	store := NewS3RequestBodyRefStore(client)
	store.Now = func() time.Time { return time.Unix(100, 0) }

	frame, err := store.UploadResponseBody(context.Background(), "ten_a", "req_1", body)
	if err != nil {
		t.Fatalf("UploadResponseBody() error = %v", err)
	}
	if got := <-putBody; got != string(body) {
		t.Fatalf("uploaded body = %q, want %q", got, string(body))
	}
	if frame.GetExpectedSizeBytes() != uint64(len(body)) || frame.GetSha256Hex() == "" {
		t.Fatalf("frame integrity fields = size %d sha %q", frame.GetExpectedSizeBytes(), frame.GetSha256Hex())
	}
	ref := frame.GetS3()
	if ref == nil || !strings.HasPrefix(ref.GetObjectKey(), "tenant/ten_a/request/req_1/response/") || ref.GetSignedUrl() == "" {
		t.Fatalf("S3 ref = %#v, want response-scoped object key and signed URL", ref)
	}
	// The download reference is a short-lived signed URL (docs/planning/18
	// Object Storage Security). Object-lifecycle retention days (1-3) are bounded
	// and tested at the objectstore foundation (task 06); here we prove the
	// signed URL itself expires quickly.
	wantExpiry := time.Unix(100, 0).Add(objectstore.DefaultPresignExpiry).UnixMilli()
	if ref.GetExpiresUnixMs() != wantExpiry {
		t.Fatalf("expires = %d, want %d", ref.GetExpiresUnixMs(), wantExpiry)
	}
}

func TestS3RequestBodyRefStoreStopsUploadOnCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		_, _ = io.Copy(io.Discard, r.Body)
		<-release
	}))
	t.Cleanup(server.Close)

	t.Setenv(testBodyRefEnvA, "id-value")
	t.Setenv(testBodyRefEnvB, "signing-value")
	client, err := objectstore.New(objectstore.Options{
		Enabled:       true,
		Endpoint:      server.URL,
		Bucket:        testBodyRefBucket,
		Region:        testBodyRefRegion,
		AccessKeyEnv:  testBodyRefEnvA,
		SecretKeyEnv:  testBodyRefEnvB,
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("objectstore.New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, uploadErr := NewS3RequestBodyRefStore(client).UploadRequestBody(ctx, "ten_a", "req_cancel", []byte("body"))
		done <- uploadErr
	}()

	<-started
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("UploadRequestBody() error = nil, want cancellation error")
		}
		close(release)
	case <-time.After(time.Second):
		close(release)
		t.Fatal("UploadRequestBody() did not stop after cancellation")
	}
}

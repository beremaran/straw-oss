package objectstore

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestS3SignsAndStreamsObjects(t *testing.T) {
	t.Parallel()
	var stored string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=access/") {
			t.Errorf("missing signature: %q", r.Header.Get("Authorization"))
		}
		switch r.Method {
		case http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			stored = string(raw)
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			if r.URL.Query().Get("list-type") == "2" {
				_, _ = io.WriteString(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>receipts/x</Key><Size>5</Size><LastModified>2026-01-01T00:00:00Z</LastModified></Contents></ListBucketResult>`)

				return
			}
			_, _ = io.WriteString(w, stored)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()
	store := S3{Endpoint: server.URL, Bucket: "bucket", Region: "us-east-1", AccessKey: "access", SecretKey: "secret", Now: func() time.Time { return time.Unix(0, 0) }}
	err := store.Put(context.Background(), "receipts/x", strings.NewReader("hello"), 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := store.Open(context.Background(), "receipts/x")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	if string(raw) != "hello" {
		t.Fatalf("body=%q", raw)
	}
	objects, err := store.List(context.Background(), "receipts/")
	if err != nil || len(objects) != 1 {
		t.Fatalf("List=%#v %v", objects, err)
	}
}

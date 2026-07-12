package receipt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/beremaran/straw-oss/v2/internal/objectstore"
)

func newTestService(t *testing.T, now *time.Time) *Service {
	t.Helper()
	service, err := New(objectstore.Local{Root: t.TempDir()}, Config{DownloadBaseURL: "http://control:8080", SigningKey: []byte("01234567890123456789012345678901"), MaxObjectBytes: 1024, MaxPartBytes: 8, Retention: time.Hour, AssignmentTTL: time.Minute, Now: func() time.Time { return *now }})
	if err != nil {
		t.Fatal(err)
	}

	return service
}

func TestRequestReceiptInterruptedUploadVerificationAndAssignmentScope(t *testing.T) {
	t.Parallel()
	now := time.Unix(1000, 0)
	service := newTestService(t, &now)
	ctx := context.Background()
	body := []byte("hello world")
	sum := sha256.Sum256(body)
	record, err := service.Create(ctx, "default", CreateInput{Direction: DirectionRequest, SizeBytes: int64(len(body)), SHA256Hex: hex.EncodeToString(sum[:]), IdempotencyKey: "same"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := service.Create(ctx, "default", CreateInput{Direction: DirectionRequest, SizeBytes: int64(len(body)), SHA256Hex: hex.EncodeToString(sum[:]), IdempotencyKey: "same"})
	if err != nil || again.ID != record.ID {
		t.Fatalf("idempotent create = %#v, %v", again, err)
	}
	_, err = service.PutPart(ctx, "default", record.ID, 1, strings.NewReader("hello "), 6, "")
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.Get(ctx, "default", record.ID)
	if err != nil || status.State != StateUploading || len(status.Parts) != 1 {
		t.Fatalf("interrupted status = %#v, %v", status, err)
	}
	_, err = service.PutPart(ctx, "default", record.ID, 2, strings.NewReader("world"), 5, "")
	if err != nil {
		t.Fatal(err)
	}
	verified, err := service.Complete(ctx, "default", record.ID)
	if err != nil || verified.State != StateVerified {
		t.Fatalf("complete = %#v, %v", verified, err)
	}
	ref, err := service.PrepareRequest(ctx, "default", record.ID, "req_1")
	if err != nil {
		t.Fatal(err)
	}
	signed, _ := url.Parse(ref.GetS3().GetSignedUrl())
	q := signed.Query()
	r, assigned, err := service.OpenAssigned(ctx, record.ID, q.Get("deployment_id"), q.Get("request_id"), q.Get("expires"), q.Get("signature"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	if string(raw) != string(body) || assigned.State != StateAssigned {
		t.Fatalf("assigned body=%q record=%#v", raw, assigned)
	}
	_, _, err = service.OpenAssigned(ctx, record.ID, "default", "req_other", q.Get("expires"), q.Get("signature"))
	if err == nil {
		t.Fatal("reference usable outside assignment")
	}
	err = service.FinishRequest(ctx, "default", record.ID, "req_1", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.PrepareRequest(ctx, "default", record.ID, "req_2")
	if err != nil {
		t.Fatalf("retry after failed assignment: %v", err)
	}
}

func TestReceiptRejectsCorruptionAndCleansExpiredParts(t *testing.T) {
	t.Parallel()
	now := time.Unix(2000, 0)
	service := newTestService(t, &now)
	ctx := context.Background()
	sum := sha256.Sum256([]byte("expected"))
	record, _ := service.Create(ctx, "default", CreateInput{Direction: DirectionRequest, SizeBytes: 8, SHA256Hex: hex.EncodeToString(sum[:])})
	_, _ = service.PutPart(ctx, "default", record.ID, 1, strings.NewReader("corrupt!"), 8, "")
	rejected, err := service.Complete(ctx, "default", record.ID)
	if err == nil || rejected.State != StateRejected || rejected.Failure != "checksum_mismatch" {
		t.Fatalf("corruption = %#v, %v", rejected, err)
	}
	record, _ = service.Create(ctx, "default", CreateInput{Direction: DirectionRequest, SizeBytes: 4, SHA256Hex: hex.EncodeToString(sum[:])})
	_, _ = service.PutPart(ctx, "default", record.ID, 1, strings.NewReader("part"), 4, "")
	now = now.Add(2 * time.Hour)
	err = service.Cleanup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expired, err := service.Get(ctx, "default", record.ID)
	if err != nil || expired.State != StateExpired {
		t.Fatalf("expired = %#v, %v", expired, err)
	}
}

func TestResponseUploadProducesDownloadableReceipt(t *testing.T) {
	t.Parallel()
	now := time.Unix(3000, 0)
	service := newTestService(t, &now)
	ctx := context.Background()
	upload, err := service.BeginResponse(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	err = upload.Write([]byte("large "))
	if err != nil {
		t.Fatal(err)
	}
	err = upload.Write([]byte("response"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := upload.Commit(ctx)
	if err != nil || record.Direction != DirectionResponse || record.State != StateVerified {
		t.Fatalf("commit = %#v, %v", record, err)
	}
	r, _, err := service.OpenResponse(ctx, "default", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(r)
	_ = r.Close()
	if string(raw) != "large response" {
		t.Fatalf("body=%q", raw)
	}
}

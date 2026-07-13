package objectstore

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestLocalRoundTripAndTraversalProtection(t *testing.T) {
	t.Parallel()
	store := Local{Root: t.TempDir()}
	err := store.Put(context.Background(), "receipts/one/body", strings.NewReader("hello"), 5, map[string]string{"sha256": "sum"})
	if err != nil {
		t.Fatal(err)
	}
	r, object, err := store.Open(context.Background(), "receipts/one/body")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	raw, _ := io.ReadAll(r)
	if string(raw) != "hello" || object.Metadata["sha256"] != "sum" {
		t.Fatalf("object = %#v body=%q", object, raw)
	}
	listed, err := store.List(context.Background(), "receipts/")
	if err != nil || len(listed) != 1 {
		t.Fatalf("List() = %#v, %v", listed, err)
	}
	err = store.Delete(context.Background(), "receipts/one/body")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = store.Open(context.Background(), "receipts/one/body")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after delete = %v", err)
	}
	err = store.Put(context.Background(), "../escape", strings.NewReader("x"), 1, nil)
	if err == nil {
		t.Fatal("traversal accepted")
	}
}

package control

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisWorkerNonceStoreConsumeFirstUseThenReplay(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisWorkerNonceStore(client)

	fresh, err := store.Consume(context.Background(), "cred_1", []byte("nonce-a"), time.Minute)
	if err != nil {
		t.Fatalf("Consume() first use error = %v", err)
	}
	if !fresh {
		t.Fatal("Consume() first use = false, want true (fresh)")
	}

	fresh, err = store.Consume(context.Background(), "cred_1", []byte("nonce-a"), time.Minute)
	if err != nil {
		t.Fatalf("Consume() replay error = %v", err)
	}
	if fresh {
		t.Fatal("Consume() replay = true, want false (already consumed)")
	}
}

func TestRedisWorkerNonceStoreScopedByCredential(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisWorkerNonceStore(client)

	nonce := []byte("shared-nonce")

	fresh, err := store.Consume(context.Background(), "cred_a", nonce, time.Minute)
	if err != nil || !fresh {
		t.Fatalf("Consume() cred_a = (%v, %v), want (true, nil)", fresh, err)
	}

	// The same nonce bytes under a different credential_id are tracked
	// independently (docs/planning/27 "scoped by credential_id").
	fresh, err = store.Consume(context.Background(), "cred_b", nonce, time.Minute)
	if err != nil || !fresh {
		t.Fatalf("Consume() cred_b = (%v, %v), want (true, nil)", fresh, err)
	}
}

func TestRedisWorkerNonceStoreTTLExpiry(t *testing.T) {
	client := newTestRedisClient(t)
	store := NewRedisWorkerNonceStore(client)

	nonce := []byte("expiring-nonce")

	fresh, err := store.Consume(context.Background(), "cred_1", nonce, 50*time.Millisecond)
	if err != nil || !fresh {
		t.Fatalf("Consume() first use = (%v, %v), want (true, nil)", fresh, err)
	}

	time.Sleep(150 * time.Millisecond)

	fresh, err = store.Consume(context.Background(), "cred_1", nonce, time.Minute)
	if err != nil || !fresh {
		t.Fatalf("Consume() after TTL expiry = (%v, %v), want (true, nil): nonces must expire", fresh, err)
	}
}

func TestRedisWorkerNonceStoreErrorsOnOutage(t *testing.T) {
	unreachable := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	t.Cleanup(func() { _ = unreachable.Close() })

	store := NewRedisWorkerNonceStore(unreachable)

	_, err := store.Consume(context.Background(), "cred_1", []byte("nonce-a"), time.Minute)
	if err == nil {
		t.Fatal("Consume() during Redis outage = nil error, want an error so the caller can apply its fail policy")
	}
}

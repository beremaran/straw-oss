package control

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/beremaran/straw/v2/internal/config"
)

// TestRedisInvalidationPublishSubscribe proves the pub/sub acceleration path
// end to end against a real Redis instance: a publish on one client reaches
// a subscriber running against a cached ConfigCache and clears its stale
// entry (docs/planning/25 "Invalidation").
func TestRedisInvalidationPublishSubscribe(t *testing.T) {
	client := newTestRedisClient(t)

	store := newFakeSnapshotStore()
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 1, nil))
	store.seedSnapshot(config.NewTenantSnapshot(testTenantA, 2, []string{testKeyA}))
	store.setCurrentVersion(testTenantA, 1)

	cache := NewConfigCache(store, nil)

	_, err := cache.Snapshot(context.Background(), testTenantA)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	subscriber := NewRedisInvalidationSubscriber(client, cache)

	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = subscriber.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		<-done
	})

	publisher := NewRedisInvalidationPublisher(client)
	store.setCurrentVersion(testTenantA, 2)

	deadline := time.Now().Add(2 * time.Second)

	for {
		err = publisher.PublishTenantInvalidation(context.Background(), testTenantA, 2)
		if err != nil {
			t.Fatalf("PublishTenantInvalidation() error = %v", err)
		}

		snapshot, snapErr := cache.Snapshot(context.Background(), testTenantA)
		if snapErr != nil {
			t.Fatalf("Snapshot() error = %v", snapErr)
		}

		if snapshot.ConfigVersion == 2 {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("Snapshot() version = %d, want 2 (subscriber never applied invalidation)", snapshot.ConfigVersion)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// TestRedisInvalidationPublisherPublishError proves a Redis outage surfaces
// as an error to the caller rather than panicking or blocking.
func TestRedisInvalidationPublisherPublishError(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: testUnreachableRedisAddr, DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	defer func() { _ = client.Close() }()

	publisher := NewRedisInvalidationPublisher(client)

	err := publisher.PublishTenantInvalidation(context.Background(), testTenantA, 1)
	if err == nil {
		t.Fatal("PublishTenantInvalidation() error = nil, want error for unreachable redis")
	}
}

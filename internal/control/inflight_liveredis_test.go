package control

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestRedisInFlightCoordinatorLive exercises the real RedisInFlightCoordinator
// and RedisRequestCancelSubscriber against a live Redis, proving the actual
// Set/Get/Del/Publish/Subscribe round-trip (docs/tasks/p1/23) — not the fake
// backend the pure-unit tests use. It only runs when STRAW_TEST_REDIS_URL is
// set (e.g. the compose Redis at redis://localhost:6379/0); otherwise skipped.
func TestRedisInFlightCoordinatorLive(t *testing.T) {
	url := os.Getenv("STRAW_TEST_REDIS_URL")
	if url == "" {
		t.Skip("STRAW_TEST_REDIS_URL not set; skipping live Redis cross-instance test")
	}

	opt, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse redis url: %v", err)
	}

	// Simulate two Control replicas over one Redis: instanceB owns the request,
	// the cancel is delivered to instanceA.
	clientA := redis.NewClient(opt)
	clientB := redis.NewClient(opt)

	t.Cleanup(func() { _ = clientA.Close(); _ = clientB.Close() })

	ctx := context.Background()

	err = clientA.Ping(ctx).Err()
	if err != nil {
		t.Skipf("live Redis unreachable: %v", err)
	}

	instanceA := NewInFlightRegistry()
	instanceB := NewInFlightRegistry()
	instanceA.SetCrossInstance(NewRedisInFlightCoordinator(clientA, time.Minute))
	instanceB.SetCrossInstance(NewRedisInFlightCoordinator(clientB, time.Minute))

	// Only instanceB runs the cancel subscriber; it owns the request.
	subCtx, stop := context.WithCancel(ctx)
	defer stop()

	subReady := make(chan struct{})
	go func() {
		sub := clientB.Subscribe(subCtx, redisRequestCancelChannel)
		defer func() { _ = sub.Close() }()

		_, rerr := sub.Receive(subCtx)
		if rerr != nil {
			return
		}

		close(subReady)

		ch := sub.Channel()
		for {
			select {
			case <-subCtx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}

				instanceB.cancelLocal(msg.Payload)
			}
		}
	}()

	select {
	case <-subReady:
	case <-time.After(3 * time.Second):
		t.Fatal("cancel subscriber did not become ready")
	}

	requestID := "req_live_" + t.Name()
	cancelled := make(chan struct{})
	instanceB.Register(ctx, requestID, inflightTestTenantA, func() { close(cancelled) })
	defer instanceB.Deregister(ctx, requestID)

	// The cancel is delivered to instanceA, which does not own the request; it
	// must resolve the owner via Redis and signal instanceB to tear it down.
	err = instanceA.Cancel(ctx, Identity{ScopeType: ScopePlatform, Role: RoleSystemAdmin}, requestID)
	if err != nil {
		t.Fatalf("cross-instance Cancel() over live Redis error = %v, want nil", err)
	}

	select {
	case <-cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("owning instance did not cancel the request via the live Redis cancel signal")
	}
}

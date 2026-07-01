package broker

import (
	"testing"

	"github.com/nats-io/nats.go"
)

func TestNewNatsBroker(t *testing.T) {
	t.Run("uses default URL when empty", func(t *testing.T) {
		b := NewNatsBroker("", "token")
		if b.url != nats.DefaultURL {
			t.Errorf("url = %q, want %q", b.url, nats.DefaultURL)
		}
		if b.token != "token" {
			t.Errorf("token = %q, want %q", b.token, "token")
		}
	})

	t.Run("uses provided URL", func(t *testing.T) {
		want := "nats://localhost:4222"
		b := NewNatsBroker(want, "mytoken")
		if b.url != want {
			t.Errorf("url = %q, want %q", b.url, want)
		}
		if b.token != "mytoken" {
			t.Errorf("token = %q, want %q", b.token, "mytoken")
		}
	})

	t.Run("empty token is allowed", func(t *testing.T) {
		b := NewNatsBroker("nats://localhost:4222", "")
		if b.token != "" {
			t.Errorf("token = %q, want empty", b.token)
		}
	})
}

func TestErrTimeout(t *testing.T) {
	if ErrTimeout == nil {
		t.Fatal("ErrTimeout should not be nil")
	}
	if ErrTimeout.Error() != "timeout" {
		t.Errorf("ErrTimeout.Error() = %q, want %q", ErrTimeout.Error(), "timeout")
	}
}

func TestErrNoStreamForSubject(t *testing.T) {
	if ErrNoStreamForSubject == nil {
		t.Fatal("ErrNoStreamForSubject should not be nil")
	}
}

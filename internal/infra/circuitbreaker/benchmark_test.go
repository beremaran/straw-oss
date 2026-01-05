package circuitbreaker

import (
	"testing"
	"time"
)

func BenchmarkCircuitBreaker_Allow(b *testing.B) {
	cb := New(Config{
		Name:             "bench",
		FailureThreshold: 100,
		ResetTimeout:     time.Minute,
	})

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.Allow()
		}
	})
}

func BenchmarkCircuitBreaker_Execute_Success(b *testing.B) {
	cb := New(Config{
		Name:             "bench",
		FailureThreshold: 100,
		ResetTimeout:     time.Minute,
	})

	fn := func() error { return nil }

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.Execute(fn)
		}
	})
}

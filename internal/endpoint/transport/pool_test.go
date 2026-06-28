package transport

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/beremaran/straw/internal/endpoint/fingerprint"
)

type mockDialer struct {
	mu        sync.Mutex
	calls     []dialCall
	connDelay time.Duration
}

type dialCall struct {
	network     string
	addr        string
	fingerprint string
}

func (m *mockDialer) dial(ctx context.Context, network, addr, fingerprint string) (net.Conn, error) {
	m.mu.Lock()
	m.calls = append(m.calls, dialCall{network, addr, fingerprint})
	m.mu.Unlock()

	if m.connDelay > 0 {
		time.Sleep(m.connDelay)
	}

	client, _ := net.Pipe()

	return client, nil
}

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()

	if cfg.MaxPoolHosts != 1000 {
		t.Errorf("expected MaxPoolHosts=1000, got %d", cfg.MaxPoolHosts)
	}
	if cfg.IdleConnsPerHost != 10 {
		t.Errorf("expected IdleConnsPerHost=10, got %d", cfg.IdleConnsPerHost)
	}
	if cfg.IdleConnTimeout != 90*time.Second {
		t.Errorf("expected IdleConnTimeout=90s, got %v", cfg.IdleConnTimeout)
	}
	if cfg.EvictionInterval != 5*time.Minute {
		t.Errorf("expected EvictionInterval=5m, got %v", cfg.EvictionInterval)
	}
}

func TestPoolConfig_WithMethods(t *testing.T) {
	cfg := DefaultPoolConfig().
		WithMaxPoolHosts(500).
		WithIdleConnsPerHost(5).
		WithIdleConnTimeout(30 * time.Second).
		WithEvictionInterval(1 * time.Minute)

	if cfg.MaxPoolHosts != 500 {
		t.Errorf("expected MaxPoolHosts=500, got %d", cfg.MaxPoolHosts)
	}
	if cfg.IdleConnsPerHost != 5 {
		t.Errorf("expected IdleConnsPerHost=5, got %d", cfg.IdleConnsPerHost)
	}
	if cfg.IdleConnTimeout != 30*time.Second {
		t.Errorf("expected IdleConnTimeout=30s, got %v", cfg.IdleConnTimeout)
	}
	if cfg.EvictionInterval != 1*time.Minute {
		t.Errorf("expected EvictionInterval=1m, got %v", cfg.EvictionInterval)
	}
}

func TestNewPooledTransport(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig()

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	if pt == nil {
		t.Fatal("expected non-nil PooledTransport")
	}

	stats := pt.Stats()
	if stats.PoolCount != 0 {
		t.Errorf("expected empty pool, got %d", stats.PoolCount)
	}
	if stats.MaxPoolHosts != 1000 {
		t.Errorf("expected MaxPoolHosts=1000, got %d", stats.MaxPoolHosts)
	}
}

func TestPooledTransport_GetTransport_Reuse(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig()

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	preset := fingerprint.Preset{ID: "chrome-133"}

	t1 := pt.GetTransport("example.com:443", preset)
	t2 := pt.GetTransport("example.com:443", preset)
	t3 := pt.GetTransport("example.com:443", preset)

	if t1 != t2 || t2 != t3 {
		t.Error("expected same transport instance for same host+fingerprint")
	}

	stats := pt.Stats()
	if stats.PoolCount != 1 {
		t.Errorf("expected 1 pool, got %d", stats.PoolCount)
	}
}

func TestPooledTransport_FingerprintIsolation(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig()

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	preset1 := fingerprint.Preset{ID: "chrome-133"}
	preset2 := fingerprint.Preset{ID: "firefox-133"}

	t1 := pt.GetTransport("example.com:443", preset1)
	t2 := pt.GetTransport("example.com:443", preset2)

	if t1 == t2 {
		t.Error("expected different transports for different fingerprints")
	}

	stats := pt.Stats()
	if stats.PoolCount != 2 {
		t.Errorf("expected 2 pools, got %d", stats.PoolCount)
	}
}

func TestPooledTransport_HostIsolation(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig()

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	preset := fingerprint.Preset{ID: "chrome-133"}

	t1 := pt.GetTransport("example.com:443", preset)
	t2 := pt.GetTransport("other.com:443", preset)

	if t1 == t2 {
		t.Error("expected different transports for different hosts")
	}

	stats := pt.Stats()
	if stats.PoolCount != 2 {
		t.Errorf("expected 2 pools, got %d", stats.PoolCount)
	}
}

func TestPooledTransport_LRUEviction(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig().WithMaxPoolHosts(3)

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	preset := fingerprint.Preset{ID: "chrome-133"}

	_ = pt.GetTransport("host1.com:443", preset)
	_ = pt.GetTransport("host2.com:443", preset)
	_ = pt.GetTransport("host3.com:443", preset)

	stats := pt.Stats()
	if stats.PoolCount != 3 {
		t.Errorf("expected 3 pools, got %d", stats.PoolCount)
	}

	_ = pt.GetTransport("host1.com:443", preset)

	_ = pt.GetTransport("host4.com:443", preset)

	stats = pt.Stats()
	if stats.PoolCount != 3 {
		t.Errorf("expected 3 pools after eviction, got %d", stats.PoolCount)
	}

	t1 := pt.GetTransport("host1.com:443", preset)
	if t1 == nil {
		t.Error("expected host1 pool to still exist")
	}
}

func TestPooledTransport_ConcurrentAccess(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig().WithMaxPoolHosts(100)

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	const goroutines = 50
	const iterations = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				host := "host.com:443"
				preset := fingerprint.Preset{ID: "chrome-133"}
				_ = pt.GetTransport(host, preset)
			}
		}(i)
	}

	wg.Wait()

	stats := pt.Stats()
	if stats.PoolCount != 1 {
		t.Errorf("expected 1 pool after concurrent access, got %d", stats.PoolCount)
	}
}

func TestPooledTransport_ConcurrentDifferentHosts(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig().WithMaxPoolHosts(100)

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	const goroutines = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			host := fmt.Sprintf("host%c.com:443", 'A'+id)
			preset := fingerprint.Preset{ID: "chrome-133"}
			_ = pt.GetTransport(host, preset)
		}(i)
	}

	wg.Wait()

	stats := pt.Stats()
	if stats.PoolCount != goroutines {
		t.Errorf("expected %d pools, got %d", goroutines, stats.PoolCount)
	}
}

func TestPooledTransport_Close(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig()

	pt := NewPooledTransport(cfg, dialer.dial)

	preset := fingerprint.Preset{ID: "chrome-133"}

	_ = pt.GetTransport("host1.com:443", preset)
	_ = pt.GetTransport("host2.com:443", preset)

	err := pt.Close()
	if err != nil {
		t.Errorf("unexpected error on close: %v", err)
	}

	stats := pt.Stats()
	if stats.PoolCount != 0 {
		t.Errorf("expected 0 pools after close, got %d", stats.PoolCount)
	}
}

func TestPooledTransport_StaleEviction(t *testing.T) {
	dialer := &mockDialer{}
	cfg := DefaultPoolConfig().
		WithIdleConnTimeout(50 * time.Millisecond).
		WithEvictionInterval(20 * time.Millisecond)

	pt := NewPooledTransport(cfg, dialer.dial)
	defer func() { _ = pt.Close() }()

	preset := fingerprint.Preset{ID: "chrome-133"}

	_ = pt.GetTransport("stale.com:443", preset)

	stats := pt.Stats()
	if stats.PoolCount != 1 {
		t.Errorf("expected 1 pool, got %d", stats.PoolCount)
	}

	time.Sleep(150 * time.Millisecond)

	stats = pt.Stats()
	if stats.PoolCount != 0 {
		t.Errorf("expected 0 pools after stale eviction, got %d", stats.PoolCount)
	}
}

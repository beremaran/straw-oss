package transport

import (
	"container/list"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/http2"
	tls "github.com/bogdanfinn/utls"

	"github.com/beremaran/straw/internal/egress/fingerprint"
)

// DialTLSFunc is a function that establishes a TLS connection with client
// fingerprinting for protocol negotiation.
type DialTLSFunc func(ctx context.Context, network, addr string, fingerprint string) (net.Conn, error)

// PooledTransport manages per-host pooled HTTP transports with LRU eviction.
type PooledTransport struct {
	pools        map[string]*hostPool
	lruList      *list.List
	lruMap       map[string]*list.Element
	mu           sync.RWMutex
	config       PoolConfig
	dialTLS      DialTLSFunc
	stopEviction chan struct{}
	evictionDone chan struct{}
}

type hostPool struct {
	key         string
	host        string
	fingerprint string
	transport   *fhttp.Transport
	lastUsed    time.Time
}

// NewPooledTransport creates a new PooledTransport with the given config and dialer.
func NewPooledTransport(config PoolConfig, dialTLS DialTLSFunc) *PooledTransport {
	pt := &PooledTransport{
		pools:        make(map[string]*hostPool),
		lruList:      list.New(),
		lruMap:       make(map[string]*list.Element),
		config:       config,
		dialTLS:      dialTLS,
		stopEviction: make(chan struct{}),
		evictionDone: make(chan struct{}),
	}

	go pt.evictionLoop()

	return pt
}

// GetTransport returns an existing or creates a new pooled transport for the
// given host and fingerprint combination.
func (pt *PooledTransport) GetTransport(host string, preset fingerprint.Preset) *fhttp.Transport {
	key := pt.makeKey(host, preset.ID)

	pt.mu.RLock()

	if pool, ok := pt.pools[key]; ok {
		pt.mu.RUnlock()
		pt.touchPool(key)

		return pool.transport
	}

	pt.mu.RUnlock()

	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pool, ok := pt.pools[key]; ok {
		pt.touchPoolLocked(key)

		return pool.transport
	}

	if len(pt.pools) >= pt.config.MaxPoolHosts {
		pt.evictOldestLocked()
	}

	transport := pt.createTransport(preset)
	pool := &hostPool{
		key:         key,
		host:        host,
		fingerprint: preset.ID,
		transport:   transport,
		lastUsed:    time.Now(),
	}

	pt.pools[key] = pool
	elem := pt.lruList.PushFront(key)
	pt.lruMap[key] = elem

	return transport
}

// Close shuts down the eviction loop and closes all pooled transports.
func (pt *PooledTransport) Close() error {
	close(pt.stopEviction)
	<-pt.evictionDone

	pt.mu.Lock()
	defer pt.mu.Unlock()

	for key, pool := range pt.pools {
		pool.transport.CloseIdleConnections()
		delete(pt.pools, key)
	}

	pt.lruList.Init()
	pt.lruMap = make(map[string]*list.Element)

	return nil
}

// Stats returns the current pool statistics.
func (pt *PooledTransport) Stats() PoolStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	return PoolStats{
		PoolCount:    len(pt.pools),
		MaxPoolHosts: pt.config.MaxPoolHosts,
	}
}

// PoolStats holds statistics about the transport pool.
type PoolStats struct {
	PoolCount    int
	MaxPoolHosts int
}

func (pt *PooledTransport) createTransport(preset fingerprint.Preset) *fhttp.Transport {
	h2Transport := &http2.Transport{
		DialTLS: func(network, addr string, _ *tls.Config) (net.Conn, error) {
			ctx := context.Background()

			return pt.dialTLS(ctx, network, addr, preset.ID)
		},
		AllowHTTP:                  false,
		DisableCompression:         true,
		StrictMaxConcurrentStreams: false,
	}

	if preset.HTTP2Settings != nil {
		h2Transport.InitialWindowSize = preset.HTTP2Settings.InitialWindowSize
		h2Transport.HeaderTableSize = preset.HTTP2Settings.HeaderTableSize
	}

	h1Transport := &fhttp.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return pt.dialTLS(ctx, network, addr, preset.ID)
		},
		MaxIdleConns:        pt.config.IdleConnsPerHost,
		MaxIdleConnsPerHost: pt.config.IdleConnsPerHost,
		IdleConnTimeout:     pt.config.IdleConnTimeout,
		ForceAttemptHTTP2:   false,
		DisableCompression:  true,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	h1Transport.RegisterProtocol("https", &h2RoundTripper{h2: h2Transport, h1: h1Transport})

	return h1Transport
}

type h2RoundTripper struct {
	h2 *http2.Transport
	h1 *fhttp.Transport
}

func (rt *h2RoundTripper) RoundTrip(req *fhttp.Request) (*fhttp.Response, error) {
	resp, err := rt.h2.RoundTrip(req)
	if err != nil {
		h1Resp, h1Err := rt.h1.RoundTrip(req)

		return h1Resp, fmt.Errorf("h1 fallback roundtrip: %w", h1Err)
	}

	return resp, nil
}

func (pt *PooledTransport) makeKey(host, fingerprint string) string {
	return host + ":" + fingerprint
}

func (pt *PooledTransport) touchPool(key string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.touchPoolLocked(key)
}

func (pt *PooledTransport) touchPoolLocked(key string) {
	if pool, ok := pt.pools[key]; ok {
		pool.lastUsed = time.Now()
	}

	if elem, ok := pt.lruMap[key]; ok {
		pt.lruList.MoveToFront(elem)
	}
}

func (pt *PooledTransport) evictOldestLocked() {
	if pt.lruList.Len() == 0 {
		return
	}

	elem := pt.lruList.Back()
	if elem == nil {
		return
	}

	key := elem.Value.(string)
	pt.removePoolLocked(key, elem)
}

func (pt *PooledTransport) removePoolLocked(key string, elem *list.Element) {
	if pool, ok := pt.pools[key]; ok {
		pool.transport.CloseIdleConnections()
		delete(pt.pools, key)
	}

	if elem != nil {
		pt.lruList.Remove(elem)
		delete(pt.lruMap, key)
	}
}

func (pt *PooledTransport) evictionLoop() {
	defer close(pt.evictionDone)

	ticker := time.NewTicker(pt.config.EvictionInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pt.stopEviction:
			return
		case <-ticker.C:
			pt.evictStale()
		}
	}
}

func (pt *PooledTransport) evictStale() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	cutoff := time.Now().Add(-pt.config.IdleConnTimeout)
	keysToRemove := make([]string, 0)

	for key, pool := range pt.pools {
		if pool.lastUsed.Before(cutoff) {
			keysToRemove = append(keysToRemove, key)
		}
	}

	for _, key := range keysToRemove {
		elem := pt.lruMap[key]
		pt.removePoolLocked(key, elem)
	}
}

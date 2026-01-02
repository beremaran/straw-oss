package transport

import (
	"container/list"
	"context"
	"crypto/tls"
	"net"
	"sync"
	"time"

	fhttp "github.com/useflyent/fhttp"
	"github.com/useflyent/fhttp/http2"

	"github.com/kwilabs/straw-proxy-server/internal/endpoint/fingerprint"
	"github.com/kwilabs/straw-proxy-server/internal/endpoint/metrics"
)

// DialTLSFunc is the function signature for TLS dial operations.
// It establishes a TLS connection with the specified fingerprint.
type DialTLSFunc func(ctx context.Context, network, addr string, fingerprint string) (net.Conn, error)

// PooledTransport manages a pool of HTTP transports keyed by host and fingerprint.
// It provides connection reuse for repeated requests to the same target while
// ensuring fingerprint isolation (different fingerprint = new connection).
type PooledTransport struct {
	pools   map[string]*hostPool     // key: "host:fingerprint"
	lruList *list.List               // LRU ordering for eviction
	lruMap  map[string]*list.Element // quick lookup for LRU updates
	mu      sync.RWMutex
	config  PoolConfig
	dialTLS DialTLSFunc

	// Eviction control
	stopEviction chan struct{}
	evictionDone chan struct{}
}

// hostPool represents a transport pool for a specific host+fingerprint combination.
type hostPool struct {
	key         string // "host:fingerprint"
	host        string
	fingerprint string
	transport   *fhttp.Transport
	lastUsed    time.Time
}

// NewPooledTransport creates a new pooled transport manager.
// The dialTLS function is used to establish TLS connections with specific fingerprints.
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

	// Start background eviction goroutine
	go pt.evictionLoop()

	return pt
}

// GetTransport returns a transport for the given host and fingerprint preset.
// If a matching transport exists, it's returned and marked as recently used.
// Otherwise, a new transport is created (potentially evicting the oldest if at limit).
func (pt *PooledTransport) GetTransport(host string, preset fingerprint.FingerprintPreset) *fhttp.Transport {
	key := pt.makeKey(host, preset.ID)

	// Fast path: check if pool exists
	pt.mu.RLock()
	if pool, ok := pt.pools[key]; ok {
		pt.mu.RUnlock()
		pt.touchPool(key)
		return pool.transport
	}
	pt.mu.RUnlock()

	// Slow path: create new pool
	pt.mu.Lock()
	defer pt.mu.Unlock()

	// Double-check after acquiring write lock
	if pool, ok := pt.pools[key]; ok {
		pt.touchPoolLocked(key)
		return pool.transport
	}

	// Evict oldest if at capacity
	if len(pt.pools) >= pt.config.MaxPoolHosts {
		pt.evictOldestLocked()
	}

	// Create new transport
	transport := pt.createTransport(host, preset)
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

// createTransport creates a new fhttp.Transport with the configured settings.
// This uses http2.Transport directly because fhttp.Transport's automatic HTTP/2
// detection doesn't work with custom TLS dialers (utls) - it expects *tls.Conn
// but utls returns *utls.UConn.
func (pt *PooledTransport) createTransport(host string, preset fingerprint.FingerprintPreset) *fhttp.Transport {
	// Create an HTTP/2 transport that uses our custom TLS dialer
	h2Transport := &http2.Transport{
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			ctx := context.Background()
			conn, err := pt.dialTLS(ctx, network, addr, preset.ID)
			if err == nil {
				// Track active connections
				hostVal, _, _ := net.SplitHostPort(addr)
				metrics.ConnectionsPooled.WithLabelValues(hostVal).Inc()
				// Wrap connection to decrement on close
				return &metricConn{Conn: conn, host: hostVal}, nil
			}
			return conn, err
		},
		AllowHTTP:                  false,
		DisableCompression:         true, // We handle decompression ourselves in response.go
		StrictMaxConcurrentStreams: false,
	}

	// Apply HTTP/2 settings from fingerprint if available
	if preset.HTTP2Settings != nil {
		h2Transport.InitialWindowSize = preset.HTTP2Settings.InitialWindowSize
		h2Transport.HeaderTableSize = preset.HTTP2Settings.HeaderTableSize
	}

	// Create fallback HTTP/1.1 transport for servers that don't support HTTP/2
	h1Transport := &fhttp.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := pt.dialTLS(ctx, network, addr, preset.ID)
			if err == nil {
				hostVal, _, _ := net.SplitHostPort(addr)
				metrics.ConnectionsPooled.WithLabelValues(hostVal).Inc()
				return &metricConn{Conn: conn, host: hostVal}, nil
			}
			return conn, err
		},
		MaxIdleConns:        pt.config.IdleConnsPerHost,
		MaxIdleConnsPerHost: pt.config.IdleConnsPerHost,
		IdleConnTimeout:     pt.config.IdleConnTimeout,
		ForceAttemptHTTP2:   false, // Disable automatic HTTP/2 for the fallback
		DisableCompression:  true,  // We handle decompression ourselves in response.go
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, //nolint:gosec // Overridden by utls in dialTLS
		},
	}

	// Register the HTTP/2 transport for "https" scheme
	// This allows the transport to use HTTP/2 for all HTTPS connections
	h1Transport.RegisterProtocol("https", &h2RoundTripper{h2: h2Transport, h1: h1Transport})

	return h1Transport
}

// h2RoundTripper wraps http2.Transport to implement http.RoundTripper
// and provides fallback to HTTP/1.1 if HTTP/2 fails.
type h2RoundTripper struct {
	h2 *http2.Transport
	h1 *fhttp.Transport
}

func (rt *h2RoundTripper) RoundTrip(req *fhttp.Request) (*fhttp.Response, error) {
	// Try HTTP/2 first
	resp, err := rt.h2.RoundTrip(req)
	if err != nil {
		// If HTTP/2 fails, it could be because the server doesn't support it
		// or ALPN negotiated HTTP/1.1. Try HTTP/1.1 as fallback.
		// Note: This is a simplified fallback; a more robust solution would
		// check the specific error type.
		return rt.h1.RoundTrip(req)
	}
	return resp, nil
}

// makeKey creates a pool key from host and fingerprint.
func (pt *PooledTransport) makeKey(host, fingerprint string) string {
	return host + ":" + fingerprint
}

// touchPool marks a pool as recently used (thread-safe).
func (pt *PooledTransport) touchPool(key string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.touchPoolLocked(key)
}

// touchPoolLocked marks a pool as recently used (caller must hold lock).
func (pt *PooledTransport) touchPoolLocked(key string) {
	if pool, ok := pt.pools[key]; ok {
		pool.lastUsed = time.Now()
	}
	if elem, ok := pt.lruMap[key]; ok {
		pt.lruList.MoveToFront(elem)
	}
}

// evictOldestLocked removes the least recently used pool (caller must hold write lock).
func (pt *PooledTransport) evictOldestLocked() {
	if pt.lruList.Len() == 0 {
		return
	}

	// Get oldest (back of list)
	elem := pt.lruList.Back()
	if elem == nil {
		return
	}

	key := elem.Value.(string)
	pt.removePoolLocked(key, elem)
}

// removePoolLocked removes a pool by key (caller must hold write lock).
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

// evictionLoop periodically removes stale pools that haven't been used recently.
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

// evictStale removes pools that have been idle longer than IdleConnTimeout.
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

// Close shuts down the pool manager, closing all idle connections.
func (pt *PooledTransport) Close() error {
	// Stop eviction loop
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

// Stats returns current pool statistics.
func (pt *PooledTransport) Stats() PoolStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	return PoolStats{
		PoolCount:    len(pt.pools),
		MaxPoolHosts: pt.config.MaxPoolHosts,
	}
}

// PoolStats contains pool statistics.
type PoolStats struct {
	PoolCount    int
	MaxPoolHosts int
}

// metricConn wraps net.Conn to track active connections.
type metricConn struct {
	net.Conn
	host string
}

func (c *metricConn) Close() error {
	metrics.ConnectionsPooled.WithLabelValues(c.host).Dec()
	return c.Conn.Close()
}

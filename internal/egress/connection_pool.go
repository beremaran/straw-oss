package egress

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	strawpb "github.com/beremaran/straw-protos-go/straw/v1"
)

type upstreamPoolKey struct {
	deploymentID       string
	resolutionMode     strawpb.DestinationResolutionMode
	scheme             string
	host               string
	port               uint32
	dialIP             string
	fingerprintProfile string
	useHTTP2           bool
}

type upstreamConnectionPool struct {
	enabled     bool
	dialContext func(context.Context, string, string) (net.Conn, error)
	maxIdle     int
	idleTimeout time.Duration
	maxLifetime time.Duration
	mu          sync.Mutex
	transports  map[upstreamPoolKey]pooledTransport
	executor    *Executor
}

type pooledTransport struct {
	createdAt time.Time
	tr        *http.Transport
}

func newUpstreamConnectionPool(opts UpstreamConnectionPoolOptions, dialContext func(context.Context, string, string) (net.Conn, error), executor *Executor) *upstreamConnectionPool {
	if !opts.Enabled {
		return nil
	}

	if opts.MaxIdleConnsPerHost <= 0 {
		opts.MaxIdleConnsPerHost = 2
	}

	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = defaultPoolIdleTimeout
	}

	if opts.MaxLifetime <= 0 {
		opts.MaxLifetime = defaultPoolMaxLifetime
	}

	return &upstreamConnectionPool{
		enabled:     true,
		dialContext: dialContext,
		maxIdle:     opts.MaxIdleConnsPerHost,
		idleTimeout: opts.IdleTimeout,
		maxLifetime: opts.MaxLifetime,
		transports:  map[upstreamPoolKey]pooledTransport{},
		executor:    executor,
	}
}

func (p *upstreamConnectionPool) transport(key upstreamPoolKey) *http.Transport {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	if pooled, ok := p.transports[key]; ok {
		if now.Sub(pooled.createdAt) < p.maxLifetime {
			return pooled.tr
		}

		pooled.tr.CloseIdleConnections()
	}

	tr := NewPooledTransport(func(ctx context.Context, network, _ string) (net.Conn, error) {
		return p.dialContext(ctx, network, net.JoinHostPort(key.dialIP, strconv.FormatUint(uint64(key.port), 10)))
	}, p.maxIdle, p.idleTimeout)

	p.executor.configureHTTP2(tr, key.useHTTP2, key.host, func() {
		p.evict(key)
	}, func(ctx context.Context, network, _ string) (net.Conn, error) {
		return p.dialContext(ctx, network, net.JoinHostPort(key.dialIP, strconv.FormatUint(uint64(key.port), 10)))
	})

	p.transports[key] = pooledTransport{createdAt: now, tr: tr}

	return tr
}

func (p *upstreamConnectionPool) evict(key upstreamPoolKey) {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if pooled, ok := p.transports[key]; ok {
		pooled.tr.CloseIdleConnections()
		delete(p.transports, key)
	}
}

func (p *upstreamConnectionPool) discardStale(current upstreamPoolKey, validIPs []netip.Addr) {
	p.mu.Lock()
	defer p.mu.Unlock()

	valid := make(map[string]struct{}, len(validIPs))
	for _, ip := range validIPs {
		valid[ip.String()] = struct{}{}
	}

	for key, pooled := range p.transports {
		if !key.samePoolExceptIP(current) {
			continue
		}

		if _, ok := valid[key.dialIP]; ok {
			continue
		}

		pooled.tr.CloseIdleConnections()
		delete(p.transports, key)
	}
}

func (p *upstreamConnectionPool) closeIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, pooled := range p.transports {
		pooled.tr.CloseIdleConnections()
		delete(p.transports, key)
	}
}

func (k upstreamPoolKey) samePoolExceptIP(other upstreamPoolKey) bool {
	return k.deploymentID == other.deploymentID &&
		k.resolutionMode == other.resolutionMode &&
		k.scheme == other.scheme &&
		k.host == other.host &&
		k.port == other.port &&
		k.fingerprintProfile == other.fingerprintProfile &&
		k.useHTTP2 == other.useHTTP2
}

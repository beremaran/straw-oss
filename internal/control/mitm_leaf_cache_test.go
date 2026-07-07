package control

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	testMITMLeafDeploymentID = "dep_leaf_test"
	testMITMLeafCAIdentity   = "ca_identity"
	testMITMLeafCAVersion    = "ca_v1"
	testMITMLeafOtherTenant  = "ten_mitm_other"
)

func TestMITMLeafCacheMissStoresEncryptedBundleAndHitReusesIt(t *testing.T) {
	client := newTestRedisClient(t)
	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
	now := time.Now().UTC()
	var generated atomic.Int32

	cache := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		Now:      func() time.Time { return now },
		Validity: 24 * time.Hour,
		Generate: func(_ context.Context, sni string) (*tls.Certificate, error) {
			generated.Add(1)

			return newTestMITMLeafCertificate(sni, now.Add(2*time.Hour))
		},
	})

	cert, err := cache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, "Example.COM.", "")
	if err != nil {
		t.Fatalf("Leaf() miss error = %v", err)
	}
	if generated.Load() != 1 {
		t.Fatalf("generated count after miss = %d, want 1", generated.Load())
	}

	aad := cache.aad(testMITMLeafTenantID, testExampleHost)
	cacheKey := cache.cacheKey(aad)
	raw, err := client.Get(context.Background(), cacheKey).Bytes()
	if err != nil {
		t.Fatalf("redis Get(%q) error = %v", cacheKey, err)
	}
	if bytes.Contains(raw, []byte("private_key_pkcs8")) || bytes.Contains(raw, []byte("PRIVATE KEY")) {
		t.Fatalf("redis value contains plaintext private key marker: %q", raw)
	}

	ttl, err := client.TTL(context.Background(), cacheKey).Result()
	if err != nil {
		t.Fatalf("TTL(%q) error = %v", cacheKey, err)
	}
	if ttl <= 0 || ttl > 2*time.Hour {
		t.Fatalf("TTL = %v, want > 0 and <= cert remaining validity", ttl)
	}

	hit, err := cache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() hit error = %v", err)
	}
	if generated.Load() != 1 {
		t.Fatalf("generated count after hit = %d, want 1", generated.Load())
	}
	if !bytes.Equal(hit.Certificate[0], cert.Certificate[0]) {
		t.Fatal("cache hit returned a different leaf certificate")
	}
}

func TestMITMLeafCacheCAAndKMSRotationBehavior(t *testing.T) {
	client := newTestRedisClient(t)
	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{
		testMITMLeafOldKeyID: []byte("old-secret"),
		testMITMLeafNewKeyID: []byte("new-secret"),
	})
	now := time.Now().UTC()
	var generated atomic.Int32
	generator := func(_ context.Context, sni string) (*tls.Certificate, error) {
		generated.Add(1)

		return newTestMITMLeafCertificate(sni, now.Add(time.Hour))
	}

	oldCache := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{KMSKeyID: testMITMLeafOldKeyID, Now: func() time.Time { return now }, Generate: generator})
	_, err := oldCache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() old key error = %v", err)
	}
	if generated.Load() != 1 {
		t.Fatalf("generated after first miss = %d, want 1", generated.Load())
	}

	overlapCache := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{KMSKeyID: testMITMLeafNewKeyID, Now: func() time.Time { return now }, Generate: generator})
	_, err = overlapCache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() overlap decrypt error = %v", err)
	}
	if generated.Load() != 1 {
		t.Fatalf("generated during KMS overlap = %d, want still 1", generated.Load())
	}

	delete(provider.keys, testMITMLeafOldKeyID)
	_, err = overlapCache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() after old key disabled error = %v", err)
	}
	if generated.Load() != 2 {
		t.Fatalf("generated after old key disabled = %d, want 2", generated.Load())
	}

	caRotated := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		KMSKeyID:  testMITMLeafNewKeyID,
		CAVersion: "ca_v2",
		Now:       func() time.Time { return now },
		Generate:  generator,
	})
	_, err = caRotated.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() after CA rotation error = %v", err)
	}
	if generated.Load() != 3 {
		t.Fatalf("generated after CA rotation = %d, want 3", generated.Load())
	}
}

func TestMITMLeafCacheRefreshesNearExpiry(t *testing.T) {
	client := newTestRedisClient(t)
	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
	now := time.Now().UTC()
	var generated atomic.Int32

	cache := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		Now:           func() time.Time { return now },
		Validity:      24 * time.Hour,
		RefreshWindow: 10 * time.Minute,
		Generate: func(_ context.Context, sni string) (*tls.Certificate, error) {
			if generated.Add(1) == 1 {
				return newTestMITMLeafCertificate(sni, now.Add(5*time.Minute))
			}

			return newTestMITMLeafCertificate(sni, now.Add(2*time.Hour))
		},
	})

	first, err := cache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() first error = %v", err)
	}

	refreshed, err := cache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() refresh error = %v", err)
	}
	if generated.Load() != 2 {
		t.Fatalf("generated count = %d, want refresh regeneration", generated.Load())
	}
	if bytes.Equal(first.Certificate[0], refreshed.Certificate[0]) {
		t.Fatal("near-expiry cache hit returned stale leaf")
	}
}

func TestMITMLeafCacheLocalSingleflightCoalescesMiss(t *testing.T) {
	client := newTestRedisClient(t)
	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
	var generated atomic.Int32

	cache := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		Generate: func(_ context.Context, sni string) (*tls.Certificate, error) {
			generated.Add(1)
			time.Sleep(100 * time.Millisecond)

			return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
		},
	})

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := cache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("Leaf() concurrent error = %v", err)
		}
	}
	if generated.Load() != 1 {
		t.Fatalf("generated count = %d, want 1", generated.Load())
	}
}

func TestMITMLeafCacheRedisLockCoalescesCrossInstanceMiss(t *testing.T) {
	client := newTestRedisClient(t)
	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
	started := make(chan struct{})
	release := make(chan struct{})
	var generatedA atomic.Int32
	var generatedB atomic.Int32

	cacheA := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		LockTTL: time.Second,
		Generate: func(_ context.Context, sni string) (*tls.Certificate, error) {
			generatedA.Add(1)
			close(started)
			<-release

			return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
		},
	})
	cacheB := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		LockTTL: time.Second,
		Generate: func(_ context.Context, sni string) (*tls.Certificate, error) {
			generatedB.Add(1)

			return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
		},
	})

	errCh := make(chan error, 2)
	go func() {
		_, err := cacheA.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
		errCh <- err
	}()
	<-started

	go func() {
		_, err := cacheB.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
		errCh <- err
	}()

	time.Sleep(100 * time.Millisecond)
	close(release)

	for range 2 {
		err := <-errCh
		if err != nil {
			t.Fatalf("Leaf() cross-instance error = %v", err)
		}
	}
	if generatedA.Load() != 1 || generatedB.Load() != 0 {
		t.Fatalf("generated A/B = %d/%d, want 1/0", generatedA.Load(), generatedB.Load())
	}
}

func TestMITMLeafCacheLockLossDoesNotBlockGeneration(t *testing.T) {
	client := newTestRedisClient(t)
	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
	var generated atomic.Int32
	cache := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		LockTTL: 75 * time.Millisecond,
		Generate: func(_ context.Context, sni string) (*tls.Certificate, error) {
			generated.Add(1)

			return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
		},
	})

	aad := cache.aad(testMITMLeafTenantID, testExampleHost)
	err := client.Set(context.Background(), cache.lockKey(aad), "lost-owner", 25*time.Millisecond).Err()
	if err != nil {
		t.Fatalf("pre-set lock error = %v", err)
	}

	_, err = cache.Leaf(context.Background(), Identity{TenantID: testMITMLeafTenantID}, testExampleHost, "")
	if err != nil {
		t.Fatalf("Leaf() after lost lock error = %v", err)
	}
	if generated.Load() != 1 {
		t.Fatalf("generated count = %d, want 1", generated.Load())
	}
}

func TestMITMLeafCacheGenerationFloodLimitsRejectBeforeSecondGeneration(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  MITMLeafCacheConfig
		ids  []Identity
	}{
		{
			name: "concurrency",
			cfg:  MITMLeafCacheConfig{MaxConcurrentGenerations: 1, MaxTenantUniqueSNIs: 10, MaxGlobalUniqueSNIs: 10},
			ids:  []Identity{{TenantID: testMITMLeafTenantID}, {TenantID: testMITMLeafTenantID}},
		},
		{
			name: "tenant unique sni",
			cfg:  MITMLeafCacheConfig{MaxConcurrentGenerations: 10, MaxTenantUniqueSNIs: 1, MaxGlobalUniqueSNIs: 10},
			ids:  []Identity{{TenantID: testMITMLeafTenantID}, {TenantID: testMITMLeafTenantID}},
		},
		{
			name: "global unique sni",
			cfg:  MITMLeafCacheConfig{MaxConcurrentGenerations: 10, MaxTenantUniqueSNIs: 10, MaxGlobalUniqueSNIs: 1},
			ids:  []Identity{{TenantID: testMITMLeafTenantID}, {TenantID: testMITMLeafOtherTenant}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestRedisClient(t)
			provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
			started := make(chan struct{})
			release := make(chan struct{})
			var generated atomic.Int32

			cfg := tt.cfg
			cfg.Generate = func(_ context.Context, sni string) (*tls.Certificate, error) {
				if generated.Add(1) == 1 {
					close(started)
					<-release
				}

				return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
			}
			cache := newTestMITMLeafCache(t, client, provider, cfg)

			errCh := make(chan error, 1)
			go func() {
				_, err := cache.Leaf(context.Background(), tt.ids[0], "one.example", "")
				errCh <- err
			}()
			<-started

			_, err := cache.Leaf(context.Background(), tt.ids[1], "two.example", "")
			if !errors.Is(err, errMITMLeafGenerationLimit) {
				close(release)
				t.Fatalf("second Leaf() error = %v, want generation limit", err)
			}
			if generated.Load() != 1 {
				close(release)
				t.Fatalf("generated before rejected second leaf = %d, want 1", generated.Load())
			}

			close(release)
			err = <-errCh
			if err != nil {
				t.Fatalf("first Leaf() error = %v", err)
			}
		})
	}
}

func TestMITMLeafCacheUniqueSNIRateLimitsRejectSequentialFloodBeforeGeneration(t *testing.T) {
	for _, tt := range []struct {
		name       string
		cfg        MITMLeafCacheConfig
		firstID    Identity
		secondID   Identity
		wantKeyTTL func(*MITMLeafCache) string
	}{
		{
			name:     "tenant rate",
			cfg:      MITMLeafCacheConfig{MaxTenantUniqueSNIsPerWindow: 1, MaxGlobalUniqueSNIsPerWindow: 10},
			firstID:  Identity{TenantID: testMITMLeafTenantID},
			secondID: Identity{TenantID: testMITMLeafTenantID},
			wantKeyTTL: func(c *MITMLeafCache) string {
				return c.tenantRateKey(testMITMLeafTenantID)
			},
		},
		{
			name:     "global rate",
			cfg:      MITMLeafCacheConfig{MaxTenantUniqueSNIsPerWindow: 10, MaxGlobalUniqueSNIsPerWindow: 1},
			firstID:  Identity{TenantID: testMITMLeafTenantID},
			secondID: Identity{TenantID: testMITMLeafOtherTenant},
			wantKeyTTL: func(c *MITMLeafCache) string {
				return c.globalRateKey()
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestRedisClient(t)
			provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
			var generated atomic.Int32

			cfg := tt.cfg
			cfg.RateWindow = time.Minute
			cfg.Generate = func(_ context.Context, sni string) (*tls.Certificate, error) {
				generated.Add(1)

				return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
			}
			cache := newTestMITMLeafCache(t, client, provider, cfg)

			_, err := cache.Leaf(context.Background(), tt.firstID, "one.example", "")
			if err != nil {
				t.Fatalf("first Leaf() error = %v", err)
			}

			_, err = cache.Leaf(context.Background(), tt.secondID, "two.example", "")
			if !errors.Is(err, errMITMLeafGenerationLimit) {
				t.Fatalf("second Leaf() error = %v, want generation limit", err)
			}

			_, err = cache.Leaf(context.Background(), tt.secondID, "two.example", "")
			if !errors.Is(err, errMITMLeafGenerationLimit) {
				t.Fatalf("second Leaf() retry error = %v, want generation limit", err)
			}
			if generated.Load() != 1 {
				t.Fatalf("generated after rejected second leaf = %d, want 1", generated.Load())
			}

			rateKey := tt.wantKeyTTL(cache)
			member, err := client.SIsMember(context.Background(), rateKey, "two.example").Result()
			if err != nil {
				t.Fatalf("rate key membership error = %v", err)
			}
			if member {
				t.Fatal("rejected SNI remained in rate set")
			}

			ttl, err := client.TTL(context.Background(), rateKey).Result()
			if err != nil {
				t.Fatalf("rate key TTL error = %v", err)
			}
			if ttl <= 0 || ttl > time.Minute {
				t.Fatalf("rate key TTL = %v, want > 0 and <= 1m", ttl)
			}
		})
	}
}

func TestMITMLeafCachePreflightRejectsUncachedUniqueSNIBeforeGeneration(t *testing.T) {
	client := newTestRedisClient(t)
	provider := newFakeMITMLeafBundleProvider(testMITMLeafKMSProvider, map[string][]byte{testKeyA: []byte("secret-a")})
	var generated atomic.Int32
	cache := newTestMITMLeafCache(t, client, provider, MITMLeafCacheConfig{
		MaxTenantUniqueSNIsPerWindow: 1,
		MaxGlobalUniqueSNIsPerWindow: 10,
		Generate: func(_ context.Context, sni string) (*tls.Certificate, error) {
			generated.Add(1)

			return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
		},
	})

	err := cache.Preflight(context.Background(), Identity{TenantID: testMITMLeafTenantID}, "one.example:443")
	if err != nil {
		t.Fatalf("first Preflight() error = %v", err)
	}

	err = cache.Preflight(context.Background(), Identity{TenantID: testMITMLeafTenantID}, "two.example:443")
	if !errors.Is(err, errMITMLeafGenerationLimit) {
		t.Fatalf("second Preflight() error = %v, want generation limit", err)
	}

	err = cache.Preflight(context.Background(), Identity{TenantID: testMITMLeafTenantID}, "two.example:443")
	if !errors.Is(err, errMITMLeafGenerationLimit) {
		t.Fatalf("second Preflight() retry error = %v, want generation limit", err)
	}
	if generated.Load() != 0 {
		t.Fatalf("generated after preflight rejection = %d, want 0", generated.Load())
	}
}

func newTestMITMLeafCache(t *testing.T, client redis.Cmdable, provider MITMLeafBundleKMSProvider, cfg MITMLeafCacheConfig) *MITMLeafCache {
	t.Helper()

	if cfg.KMSKeyID == "" {
		cfg.KMSKeyID = testKeyA
	}
	if cfg.DeploymentID == "" {
		cfg.DeploymentID = testMITMLeafDeploymentID
	}
	if cfg.CAIdentity == "" {
		cfg.CAIdentity = testMITMLeafCAIdentity
	}
	if cfg.CAVersion == "" {
		cfg.CAVersion = testMITMLeafCAVersion
	}
	if cfg.Validity == 0 {
		cfg.Validity = time.Hour
	}
	if cfg.Generate == nil {
		cfg.Generate = func(_ context.Context, sni string) (*tls.Certificate, error) {
			return newTestMITMLeafCertificate(sni, time.Now().Add(time.Hour))
		}
	}
	cfg.Redis = client
	cfg.KMS = provider

	cache, err := NewMITMLeafCache(cfg)
	if err != nil {
		t.Fatalf("NewMITMLeafCache() error = %v", err)
	}

	return cache
}

func newTestMITMLeafCertificate(host string, notAfter time.Time) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate test leaf key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate test leaf serial: %w", err)
	}

	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host},
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create test leaf cert: %w", err)
	}

	return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: tpl}, nil
}

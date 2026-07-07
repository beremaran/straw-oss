package control

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	mitmLeafCacheVersion = 1

	mitmLeafCacheKeyPrefix = "straw:mitmleaf:v1:"
	mitmLeafLockKeyPrefix  = "straw:mitmleaflock:v1:"
	mitmLeafRateKeyPrefix  = "straw:mitmleafrate:v1:"

	mitmLeafRedisOpTimeout               = 500 * time.Millisecond
	mitmLeafDefaultLockTTL               = 5 * time.Second
	mitmLeafLockPoll                     = 25 * time.Millisecond
	mitmLeafDefaultRateTTL               = time.Minute
	mitmLeafMaxRefreshTTL                = time.Hour
	mitmLeafRefreshWindowValidityDivisor = 10

	mitmLeafDefaultMaxConcurrentGenerations = 4
	mitmLeafDefaultMaxTenantUniqueSNIs      = 16
	mitmLeafDefaultMaxGlobalUniqueSNIs      = 64
)

var (
	errMITMLeafCacheConfig      = errors.New("mitm leaf cache config invalid")
	errMITMLeafCacheUnavailable = errors.New("mitm leaf cache unavailable")
	errMITMLeafGenerationLimit  = errors.New("mitm leaf generation limit exceeded")
)

// MITMLeafGenerator creates one generated leaf certificate for a normalized
// SNI. The cache owns encryption/storage and calls this only after cache,
// lock, and flood controls allow generation.
type MITMLeafGenerator func(ctx context.Context, normalizedSNI string) (*tls.Certificate, error)

// MITMLeafCacheConfig wires the P2 Redis/KMS leaf cache.
type MITMLeafCacheConfig struct {
	Redis         redis.Cmdable
	KMS           MITMLeafBundleKMSProvider
	KMSKeyID      string
	Generate      MITMLeafGenerator
	Now           func() time.Time
	LockTTL       time.Duration
	Validity      time.Duration
	RateWindow    time.Duration
	RefreshWindow time.Duration
	DeploymentID  string
	CAIdentity    string
	CAVersion     string

	MaxConcurrentGenerations     int
	MaxTenantUniqueSNIs          int // active per-tenant unique-SNI generations
	MaxGlobalUniqueSNIs          int // active process-wide unique-SNI generations
	MaxTenantUniqueSNIsPerWindow int
	MaxGlobalUniqueSNIsPerWindow int
}

// MITMLeafCache stores encrypted generated MITM leaf bundles in Redis and
// coalesces same-tenant/deployment/SNI misses before key generation.
type MITMLeafCache struct {
	redis         redis.Cmdable
	kms           MITMLeafBundleKMSProvider
	kmsKeyID      string
	generate      MITMLeafGenerator
	now           func() time.Time
	lockTTL       time.Duration
	validity      time.Duration
	rateTTL       time.Duration
	refreshWindow time.Duration

	deploymentID string
	caIdentity   string
	caVersion    string

	sem chan struct{}

	maxTenantUniqueSNIs          int
	maxGlobalUniqueSNIs          int
	maxTenantUniqueSNIsPerWindow int
	maxGlobalUniqueSNIsPerWindow int

	mu           sync.Mutex
	flights      map[string]*mitmLeafFlight
	tenantActive map[string]map[string]struct{}
	globalActive map[string]struct{}
}

type mitmLeafFlight struct {
	done chan struct{}
	cert *tls.Certificate
	err  error
}

type mitmLeafStoredValue struct {
	Version      int                    `json:"version"`
	Envelope     MITMLeafBundleEnvelope `json:"envelope"`
	NotAfterUnix int64                  `json:"not_after_unix"`
}

type mitmLeafPlainBundle struct {
	CertificateChain [][]byte `json:"certificate_chain"`
	PrivateKeyPKCS8  []byte   `json:"private_key_pkcs8"`
}

// NewMITMLeafCache validates and builds a Redis-backed MITM leaf cache.
func NewMITMLeafCache(cfg MITMLeafCacheConfig) (*MITMLeafCache, error) {
	cfg, err := normalizeMITMLeafCacheConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &MITMLeafCache{
		redis:                        cfg.Redis,
		kms:                          cfg.KMS,
		kmsKeyID:                     cfg.KMSKeyID,
		generate:                     cfg.Generate,
		now:                          cfg.Now,
		lockTTL:                      cfg.LockTTL,
		validity:                     cfg.Validity,
		rateTTL:                      cfg.RateWindow,
		refreshWindow:                cfg.RefreshWindow,
		deploymentID:                 cfg.DeploymentID,
		caIdentity:                   cfg.CAIdentity,
		caVersion:                    cfg.CAVersion,
		sem:                          make(chan struct{}, cfg.MaxConcurrentGenerations),
		maxTenantUniqueSNIs:          cfg.MaxTenantUniqueSNIs,
		maxGlobalUniqueSNIs:          cfg.MaxGlobalUniqueSNIs,
		maxTenantUniqueSNIsPerWindow: cfg.MaxTenantUniqueSNIsPerWindow,
		maxGlobalUniqueSNIsPerWindow: cfg.MaxGlobalUniqueSNIsPerWindow,
		flights:                      make(map[string]*mitmLeafFlight),
		tenantActive:                 make(map[string]map[string]struct{}),
		globalActive:                 make(map[string]struct{}),
	}, nil
}

func normalizeMITMLeafCacheConfig(cfg MITMLeafCacheConfig) (MITMLeafCacheConfig, error) {
	cfg.trim()

	err := cfg.validate()
	if err != nil {
		return MITMLeafCacheConfig{}, err
	}

	cfg.applyDefaults()

	return cfg, nil
}

func (cfg *MITMLeafCacheConfig) trim() {
	cfg.DeploymentID = strings.TrimSpace(cfg.DeploymentID)
	cfg.CAIdentity = strings.TrimSpace(cfg.CAIdentity)
	cfg.CAVersion = strings.TrimSpace(cfg.CAVersion)
	cfg.KMSKeyID = strings.TrimSpace(cfg.KMSKeyID)
}

func (cfg MITMLeafCacheConfig) validate() error {
	if cfg.Redis == nil || cfg.KMS == nil || cfg.Generate == nil || cfg.KMSKeyID == "" ||
		cfg.DeploymentID == "" || cfg.CAIdentity == "" || cfg.CAVersion == "" || cfg.Validity <= 0 {
		return errMITMLeafCacheConfig
	}

	return nil
}

func (cfg *MITMLeafCacheConfig) applyDefaults() {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	cfg.applyTimingDefaults()
	cfg.applyGenerationLimitDefaults()
}

func (cfg *MITMLeafCacheConfig) applyTimingDefaults() {
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = mitmLeafDefaultLockTTL
	}

	if cfg.RateWindow <= 0 {
		cfg.RateWindow = mitmLeafDefaultRateTTL
	}

	if cfg.RefreshWindow <= 0 {
		cfg.RefreshWindow = defaultMITMLeafRefreshWindow(cfg.Validity)
	}
}

func (cfg *MITMLeafCacheConfig) applyGenerationLimitDefaults() {
	if cfg.MaxConcurrentGenerations <= 0 {
		cfg.MaxConcurrentGenerations = mitmLeafDefaultMaxConcurrentGenerations
	}

	if cfg.MaxTenantUniqueSNIs <= 0 {
		cfg.MaxTenantUniqueSNIs = mitmLeafDefaultMaxTenantUniqueSNIs
	}

	if cfg.MaxGlobalUniqueSNIs <= 0 {
		cfg.MaxGlobalUniqueSNIs = mitmLeafDefaultMaxGlobalUniqueSNIs
	}

	if cfg.MaxTenantUniqueSNIsPerWindow <= 0 {
		cfg.MaxTenantUniqueSNIsPerWindow = cfg.MaxTenantUniqueSNIs
	}

	if cfg.MaxGlobalUniqueSNIsPerWindow <= 0 {
		cfg.MaxGlobalUniqueSNIsPerWindow = cfg.MaxGlobalUniqueSNIs
	}
}

func defaultMITMLeafRefreshWindow(validity time.Duration) time.Duration {
	window := min(mitmLeafMaxRefreshTTL, validity/mitmLeafRefreshWindowValidityDivisor)
	if window <= 0 {
		return validity
	}

	return window
}

// Leaf returns a cached or newly generated leaf for the CONNECT-authenticated
// tenant identity. Redis/KMS failures fail closed because this cache is the
// only P2 private-key storage path.
func (c *MITMLeafCache) Leaf(ctx context.Context, identity Identity, sni, authority string) (*tls.Certificate, error) {
	if identity.TenantID == "" {
		return nil, fmt.Errorf("%w: missing tenant identity", errMITMLeafCacheConfig)
	}

	normalizedSNI := normalizeMITMLeafSNI(sni, authority)
	if normalizedSNI == "" {
		return nil, fmt.Errorf("%w: missing sni", errMITMLeafCacheConfig)
	}

	aad := c.aad(identity.TenantID, normalizedSNI)
	cacheKey := c.cacheKey(aad)
	lockKey := c.lockKey(aad)

	cert, ok, err := c.load(ctx, cacheKey, aad)
	if err != nil {
		return nil, err
	}

	if ok {
		return cert, nil
	}

	return c.singleflight(cacheKey, func() (*tls.Certificate, error) {
		cert, ok, err := c.load(ctx, cacheKey, aad)
		if err != nil || ok {
			return cert, err
		}

		return c.generateWithLock(ctx, identity.TenantID, normalizedSNI, cacheKey, lockKey, aad)
	})
}

// Preflight checks an authenticated CONNECT target before the tunnel is
// established. Cached leaves bypass flood accounting; uncached unique SNIs
// consume the same Redis rate window that protects generation.
func (c *MITMLeafCache) Preflight(ctx context.Context, identity Identity, authority string) error {
	if identity.TenantID == "" {
		return fmt.Errorf("%w: missing tenant identity", errMITMLeafCacheConfig)
	}

	normalizedSNI := normalizeMITMLeafSNI("", authority)
	if normalizedSNI == "" {
		return fmt.Errorf("%w: missing sni", errMITMLeafCacheConfig)
	}

	aad := c.aad(identity.TenantID, normalizedSNI)

	_, ok, err := c.load(ctx, c.cacheKey(aad), aad)
	if err != nil || ok {
		return err
	}

	return c.checkUniqueSNIRate(ctx, identity.TenantID, normalizedSNI)
}

func (c *MITMLeafCache) generateWithLock(ctx context.Context, tenantID, normalizedSNI, cacheKey, lockKey string, aad MITMLeafBundleAAD) (*tls.Certificate, error) {
	for {
		token, locked, err := c.claimLock(ctx, lockKey)
		if err != nil {
			return nil, err
		}

		if locked {
			defer c.releaseLock(context.WithoutCancel(ctx), lockKey, token)

			err = c.checkUniqueSNIRate(ctx, tenantID, normalizedSNI)
			if err != nil {
				return nil, err
			}

			release, err := c.acquireGeneration(tenantID, normalizedSNI)
			if err != nil {
				return nil, err
			}
			defer release()

			cert, err := c.generate(ctx, normalizedSNI)
			if err != nil {
				return nil, fmt.Errorf("generate mitm leaf: %w", err)
			}

			err = c.store(ctx, cacheKey, aad, cert)
			if err != nil {
				return nil, err
			}

			return cert, nil
		}

		cert, ok, err := c.waitForPeerFill(ctx, cacheKey, aad)
		if err != nil {
			return nil, err
		}

		if ok {
			return cert, nil
		}
	}
}

func (c *MITMLeafCache) singleflight(key string, fn func() (*tls.Certificate, error)) (*tls.Certificate, error) {
	c.mu.Lock()
	if f := c.flights[key]; f != nil {
		c.mu.Unlock()
		<-f.done

		return f.cert, f.err
	}

	f := &mitmLeafFlight{done: make(chan struct{})}
	c.flights[key] = f
	c.mu.Unlock()

	f.cert, f.err = fn()

	c.mu.Lock()
	delete(c.flights, key)
	close(f.done)
	c.mu.Unlock()

	return f.cert, f.err
}

func (c *MITMLeafCache) acquireGeneration(tenantID, normalizedSNI string) (func(), error) {
	select {
	case c.sem <- struct{}{}:
	default:
		return nil, errMITMLeafGenerationLimit
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	tenantSet := c.tenantActive[tenantID]
	if tenantSet == nil {
		tenantSet = make(map[string]struct{})
		c.tenantActive[tenantID] = tenantSet
	}

	_, tenantAlready := tenantSet[normalizedSNI]
	_, globalAlready := c.globalActive[normalizedSNI]

	if !tenantAlready && len(tenantSet) >= c.maxTenantUniqueSNIs {
		<-c.sem

		return nil, errMITMLeafGenerationLimit
	}

	if !globalAlready && len(c.globalActive) >= c.maxGlobalUniqueSNIs {
		<-c.sem

		return nil, errMITMLeafGenerationLimit
	}

	tenantSet[normalizedSNI] = struct{}{}
	c.globalActive[normalizedSNI] = struct{}{}

	return func() {
		c.mu.Lock()
		delete(tenantSet, normalizedSNI)

		if len(tenantSet) == 0 {
			delete(c.tenantActive, tenantID)
		}

		delete(c.globalActive, normalizedSNI)
		c.mu.Unlock()

		<-c.sem
	}, nil
}

func (c *MITMLeafCache) load(ctx context.Context, cacheKey string, aad MITMLeafBundleAAD) (*tls.Certificate, bool, error) {
	opCtx, cancel := context.WithTimeout(ctx, mitmLeafRedisOpTimeout)
	defer cancel()

	raw, err := c.redis.Get(opCtx, cacheKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("%w: read redis leaf cache: %w", errMITMLeafCacheUnavailable, err)
	}

	var stored mitmLeafStoredValue

	err = json.Unmarshal(raw, &stored)
	if err != nil || stored.Version != mitmLeafCacheVersion {
		_ = c.deleteCacheKey(context.WithoutCancel(ctx), cacheKey)

		return nil, false, nil
	}

	plaintext, err := c.kms.DecryptMITMLeafBundle(ctx, stored.Envelope, aad)
	if err != nil {
		_ = c.deleteCacheKey(context.WithoutCancel(ctx), cacheKey)

		return nil, false, nil
	}

	cert, err := decodeMITMLeafBundle(plaintext)
	if err != nil {
		_ = c.deleteCacheKey(context.WithoutCancel(ctx), cacheKey)

		return nil, false, nil
	}

	if cert.Leaf == nil || c.leafStale(cert.Leaf) {
		_ = c.deleteCacheKey(context.WithoutCancel(ctx), cacheKey)

		return nil, false, nil
	}

	return cert, true, nil
}

func (c *MITMLeafCache) leafStale(leaf *x509.Certificate) bool {
	remaining := leaf.NotAfter.Sub(c.now())
	if remaining <= 0 {
		return true
	}

	return c.refreshWindow > 0 && remaining <= c.refreshWindow
}

func (c *MITMLeafCache) store(ctx context.Context, cacheKey string, aad MITMLeafBundleAAD, cert *tls.Certificate) error {
	plaintext, err := encodeMITMLeafBundle(cert)
	if err != nil {
		return err
	}

	env, err := c.kms.EncryptMITMLeafBundle(ctx, c.kmsKeyID, aad, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt mitm leaf bundle: %w", err)
	}

	ttl := c.cacheTTL(cert)
	if ttl <= 0 {
		return fmt.Errorf("%w: generated leaf is expired", errMITMLeafCacheConfig)
	}

	stored, err := json.Marshal(mitmLeafStoredValue{
		Version:      mitmLeafCacheVersion,
		Envelope:     env,
		NotAfterUnix: cert.Leaf.NotAfter.Unix(),
	})
	if err != nil {
		return fmt.Errorf("marshal mitm leaf cache value: %w", err)
	}

	opCtx, cancel := context.WithTimeout(ctx, mitmLeafRedisOpTimeout)
	defer cancel()

	err = c.redis.Set(opCtx, cacheKey, stored, ttl).Err()
	if err != nil {
		return fmt.Errorf("%w: write redis leaf cache: %w", errMITMLeafCacheUnavailable, err)
	}

	return nil
}

func (c *MITMLeafCache) cacheTTL(cert *tls.Certificate) time.Duration {
	if cert == nil || cert.Leaf == nil {
		return 0
	}

	ttl := cert.Leaf.NotAfter.Sub(c.now())

	return min(ttl, c.validity)
}

func (c *MITMLeafCache) claimLock(ctx context.Context, lockKey string) (string, bool, error) {
	token, err := randomMITMLeafLockToken()
	if err != nil {
		return "", false, err
	}

	opCtx, cancel := context.WithTimeout(ctx, mitmLeafRedisOpTimeout)
	defer cancel()

	ok, err := c.redis.SetNX(opCtx, lockKey, token, c.lockTTL).Result()
	if err != nil {
		return "", false, fmt.Errorf("%w: claim redis leaf lock: %w", errMITMLeafCacheUnavailable, err)
	}

	return token, ok, nil
}

func (c *MITMLeafCache) releaseLock(ctx context.Context, lockKey, token string) {
	opCtx, cancel := context.WithTimeout(ctx, mitmLeafRedisOpTimeout)
	defer cancel()

	got, err := c.redis.Get(opCtx, lockKey).Result()
	if err == nil && got == token {
		_ = c.redis.Del(opCtx, lockKey).Err()
	}
}

func (c *MITMLeafCache) waitForPeerFill(ctx context.Context, cacheKey string, aad MITMLeafBundleAAD) (*tls.Certificate, bool, error) {
	timer := time.NewTimer(c.lockTTL)
	defer timer.Stop()

	ticker := time.NewTicker(mitmLeafLockPoll)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, false, fmt.Errorf("wait for mitm leaf peer fill: %w", ctx.Err())
		case <-timer.C:
			return nil, false, nil
		case <-ticker.C:
			cert, ok, err := c.load(ctx, cacheKey, aad)
			if err != nil || ok {
				return cert, ok, err
			}
		}
	}
}

func (c *MITMLeafCache) deleteCacheKey(ctx context.Context, cacheKey string) error {
	opCtx, cancel := context.WithTimeout(ctx, mitmLeafRedisOpTimeout)
	defer cancel()

	err := c.redis.Del(opCtx, cacheKey).Err()
	if err != nil {
		return fmt.Errorf("delete mitm leaf cache key: %w", err)
	}

	return nil
}

func (c *MITMLeafCache) aad(tenantID, normalizedSNI string) MITMLeafBundleAAD {
	return MITMLeafBundleAAD{
		TenantID:      tenantID,
		DeploymentID:  c.deploymentID,
		NormalizedSNI: normalizedSNI,
		CAIdentity:    c.caIdentity,
		CAVersion:     c.caVersion,
	}
}

func (c *MITMLeafCache) cacheKey(aad MITMLeafBundleAAD) string {
	return mitmLeafCacheKeyPrefix + mitmLeafKeySuffix(aad)
}

func (c *MITMLeafCache) lockKey(aad MITMLeafBundleAAD) string {
	return mitmLeafLockKeyPrefix + mitmLeafKeySuffix(aad)
}

func (c *MITMLeafCache) tenantRateKey(tenantID string) string {
	return mitmLeafRateKeyPrefix + "tenant:" + base64.RawURLEncoding.EncodeToString([]byte(c.deploymentID)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(tenantID))
}

func (c *MITMLeafCache) globalRateKey() string {
	return mitmLeafRateKeyPrefix + "global:" + base64.RawURLEncoding.EncodeToString([]byte(c.deploymentID))
}

func (c *MITMLeafCache) checkUniqueSNIRate(ctx context.Context, tenantID, normalizedSNI string) error {
	err := c.checkUniqueSNIRateKey(ctx, c.tenantRateKey(tenantID), normalizedSNI, c.maxTenantUniqueSNIsPerWindow)
	if err != nil {
		return err
	}

	return c.checkUniqueSNIRateKey(ctx, c.globalRateKey(), normalizedSNI, c.maxGlobalUniqueSNIsPerWindow)
}

func (c *MITMLeafCache) checkUniqueSNIRateKey(ctx context.Context, key, normalizedSNI string, limit int) error {
	opCtx, cancel := context.WithTimeout(ctx, mitmLeafRedisOpTimeout)
	defer cancel()

	pipe := c.redis.TxPipeline()
	added := pipe.SAdd(opCtx, key, normalizedSNI)
	pipe.Expire(opCtx, key, c.rateTTL)
	count := pipe.SCard(opCtx, key)

	_, err := pipe.Exec(opCtx)
	if err != nil {
		return fmt.Errorf("%w: update mitm leaf unique-sni rate: %w", errMITMLeafCacheUnavailable, err)
	}

	if count.Val() > int64(limit) {
		if added.Val() > 0 {
			_ = c.removeUniqueSNIRateMember(context.WithoutCancel(ctx), key, normalizedSNI)
		}

		return errMITMLeafGenerationLimit
	}

	return nil
}

func (c *MITMLeafCache) removeUniqueSNIRateMember(ctx context.Context, key, normalizedSNI string) error {
	opCtx, cancel := context.WithTimeout(ctx, mitmLeafRedisOpTimeout)
	defer cancel()

	err := c.redis.SRem(opCtx, key, normalizedSNI).Err()
	if err != nil {
		return fmt.Errorf("%w: roll back mitm leaf unique-sni rate: %w", errMITMLeafCacheUnavailable, err)
	}

	return nil
}

func mitmLeafKeySuffix(aad MITMLeafBundleAAD) string {
	parts := []string{aad.TenantID, aad.DeploymentID, aad.NormalizedSNI, aad.CAIdentity, aad.CAVersion}
	for i, part := range parts {
		parts[i] = base64.RawURLEncoding.EncodeToString([]byte(part))
	}

	return strings.Join(parts, ":")
}

func normalizeMITMLeafSNI(sni, authority string) string {
	normalized := normalizeMITMHostNameOnly(sni)
	if normalized != "" {
		return normalized
	}

	return normalizeMITMHostNameOnly(authority)
}

func encodeMITMLeafBundle(cert *tls.Certificate) ([]byte, error) {
	if cert == nil || len(cert.Certificate) == 0 || cert.PrivateKey == nil {
		return nil, fmt.Errorf("%w: incomplete generated leaf", errMITMLeafCacheConfig)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("marshal mitm leaf private key: %w", err)
	}

	raw, err := json.Marshal(mitmLeafPlainBundle{
		CertificateChain: cert.Certificate,
		PrivateKeyPKCS8:  keyDER,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal mitm leaf bundle: %w", err)
	}

	return raw, nil
}

func decodeMITMLeafBundle(raw []byte) (*tls.Certificate, error) {
	var bundle mitmLeafPlainBundle

	err := json.Unmarshal(raw, &bundle)
	if err != nil {
		return nil, fmt.Errorf("decode mitm leaf bundle: %w", err)
	}

	if len(bundle.CertificateChain) == 0 || len(bundle.PrivateKeyPKCS8) == 0 {
		return nil, fmt.Errorf("%w: incomplete stored leaf", errMITMLeafCacheConfig)
	}

	key, err := x509.ParsePKCS8PrivateKey(bundle.PrivateKeyPKCS8)
	if err != nil {
		return nil, fmt.Errorf("parse mitm leaf private key: %w", err)
	}

	leaf, err := x509.ParseCertificate(bundle.CertificateChain[0])
	if err != nil {
		return nil, fmt.Errorf("parse mitm leaf cert: %w", err)
	}

	return &tls.Certificate{Certificate: bundle.CertificateChain, PrivateKey: key, Leaf: leaf}, nil
}

func randomMITMLeafLockToken() (string, error) {
	var raw [16]byte

	_, err := rand.Read(raw[:])
	if err != nil {
		return "", fmt.Errorf("generate mitm leaf lock token: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

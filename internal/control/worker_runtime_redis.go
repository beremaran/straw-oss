package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	strawpb "github.com/beremaran/straw/v2/api/proto/straw/v1"
)

const (
	workerRuntimeOpTimeout = 250 * time.Millisecond
	workerRuntimeKeyPrefix = "straw:worker-runtime:"
	workerRuntimeScanCount = 100
)

// RedisWorkerRuntimeStore stores worker session, heartbeat/load, duplicate
// session, failure-window, and cooldown runtime state in Redis. Every worker
// key is written with a TTL; Redis loss only removes ephemeral availability.
type RedisWorkerRuntimeStore struct {
	client redis.Cmdable
}

// NewRedisWorkerRuntimeStore builds a Redis-backed worker runtime store.
func NewRedisWorkerRuntimeStore(client redis.Cmdable) *RedisWorkerRuntimeStore {
	return &RedisWorkerRuntimeStore{client: client}
}

func workerRuntimeKey(workerID string) string {
	return workerRuntimeKeyPrefix + workerID
}

// save writes one worker runtime snapshot with ttl.
func (s *RedisWorkerRuntimeStore) save(workerID string, e *workerEntry, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), workerRuntimeOpTimeout)
	defer cancel()

	b, err := json.Marshal(workerRuntimeRecordFromEntry(e))
	if err != nil {
		return fmt.Errorf("marshal worker runtime: %w", err)
	}

	err = s.client.Set(ctx, workerRuntimeKey(workerID), b, ttl).Err()
	if err != nil {
		return fmt.Errorf("save worker runtime: %w", err)
	}

	return nil
}

// loadAll returns all unexpired worker runtime snapshots. A Redis error is
// returned so WorkerRegistry can fall back to its local snapshot.
func (s *RedisWorkerRuntimeStore) loadAll() (map[string]*workerEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), workerRuntimeOpTimeout)
	defer cancel()

	out := make(map[string]*workerEntry)

	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, workerRuntimeKeyPrefix+"*", workerRuntimeScanCount).Result()
		if err != nil {
			return nil, fmt.Errorf("scan worker runtime: %w", err)
		}

		for _, key := range keys {
			raw, err := s.client.Get(ctx, key).Bytes()
			if err != nil {
				if errors.Is(err, redis.Nil) {
					continue
				}

				return nil, fmt.Errorf("load worker runtime: %w", err)
			}

			var rec workerRuntimeRecord

			err = json.Unmarshal(raw, &rec)
			if err != nil {
				return nil, fmt.Errorf("decode worker runtime: %w", err)
			}

			out[strings.TrimPrefix(key, workerRuntimeKeyPrefix)] = rec.entry()
		}

		if next == 0 {
			return out, nil
		}

		cursor = next
	}
}

type workerRuntimeRecord struct {
	Current       *runtimeSessionRecord `json:"current,omitempty"`
	Superseded    *runtimeSessionRecord `json:"superseded,omitempty"`
	FailuresUnix  []int64               `json:"failures_unix,omitempty"`
	CooldownUnix  int64                 `json:"cooldown_unix,omitempty"`
	CooldownNanos int64                 `json:"cooldown_nanos,omitempty"`
}

type runtimeSessionRecord struct {
	SessionID                    string        `json:"session_id"`
	ExecutorType                 string        `json:"executor_type"`
	CredentialID                 string        `json:"credential_id"`
	TenantScope                  []string      `json:"tenant_scope"`
	Pools                        []AllowedPool `json:"pools,omitempty"`
	Tags                         []string      `json:"tags,omitempty"`
	Countries                    []string      `json:"countries,omitempty"`
	Regions                      []string      `json:"regions,omitempty"`
	IPTypes                      []string      `json:"ip_types,omitempty"`
	IngressModes                 []string      `json:"ingress_modes,omitempty"`
	SupportedFingerprintProfiles []string      `json:"supported_fingerprint_profiles,omitempty"`
	MaxConcurrency               uint32        `json:"max_concurrency"`
	Health                       int32         `json:"health"`
	HasHeartbeat                 bool          `json:"has_heartbeat"`
	ActiveRequests               uint32        `json:"active_requests"`
	AvailableCap                 uint32        `json:"available_cap"`
	Draining                     bool          `json:"draining"`
	RegisteredUnix               int64         `json:"registered_unix"`
	RegisteredNano               int64         `json:"registered_nano"`
	HeartbeatUnix                int64         `json:"heartbeat_unix"`
	HeartbeatNano                int64         `json:"heartbeat_nano"`
}

func workerRuntimeRecordFromEntry(e *workerEntry) workerRuntimeRecord {
	rec := workerRuntimeRecord{
		Current:      runtimeSessionRecordFromSession(e.current),
		Superseded:   runtimeSessionRecordFromSession(e.superseded),
		FailuresUnix: make([]int64, 0, len(e.failures)),
	}

	for _, failure := range e.failures {
		rec.FailuresUnix = append(rec.FailuresUnix, failure.UnixNano())
	}

	if !e.cooldownUntil.IsZero() {
		rec.CooldownUnix = e.cooldownUntil.Unix()
		rec.CooldownNanos = int64(e.cooldownUntil.Nanosecond())
	}

	return rec
}

func runtimeSessionRecordFromSession(s *runtimeSession) *runtimeSessionRecord {
	if s == nil {
		return nil
	}

	return &runtimeSessionRecord{
		SessionID:                    s.sessionID,
		ExecutorType:                 s.executorType,
		CredentialID:                 s.credentialID,
		TenantScope:                  append([]string(nil), s.tenantScope...),
		Pools:                        append([]AllowedPool(nil), s.pools...),
		Tags:                         append([]string(nil), s.tags...),
		Countries:                    append([]string(nil), s.countries...),
		Regions:                      append([]string(nil), s.regions...),
		IPTypes:                      append([]string(nil), s.ipTypes...),
		IngressModes:                 append([]string(nil), s.ingressModes...),
		SupportedFingerprintProfiles: append([]string(nil), s.supportedFingerprintProfiles...),
		MaxConcurrency:               s.maxConcurrency,
		Health:                       int32(s.health),
		HasHeartbeat:                 s.hasHeartbeat,
		ActiveRequests:               s.activeRequests,
		AvailableCap:                 s.availableCap,
		Draining:                     s.draining,
		RegisteredUnix:               s.registeredAt.Unix(),
		RegisteredNano:               int64(s.registeredAt.Nanosecond()),
		HeartbeatUnix:                s.lastHeartbeat.Unix(),
		HeartbeatNano:                int64(s.lastHeartbeat.Nanosecond()),
	}
}

func (r workerRuntimeRecord) entry() *workerEntry {
	e := &workerEntry{
		globalAdmin:   AdminEnabled,
		tenantAdmin:   make(map[string]AdminState),
		tenantDrain:   make(map[string]bool),
		failures:      make([]time.Time, 0, len(r.FailuresUnix)),
		cooldownUntil: time.Unix(r.CooldownUnix, r.CooldownNanos),
	}
	if r.Current != nil {
		e.current = r.Current.session()
	}

	if r.Superseded != nil {
		e.superseded = r.Superseded.session()
	}

	for _, failure := range r.FailuresUnix {
		e.failures = append(e.failures, time.Unix(0, failure))
	}

	return e
}

func (r *runtimeSessionRecord) session() *runtimeSession {
	if r == nil {
		return nil
	}

	return &runtimeSession{
		sessionID:                    r.SessionID,
		executorType:                 r.ExecutorType,
		credentialID:                 r.CredentialID,
		tenantScope:                  append([]string(nil), r.TenantScope...),
		pools:                        append([]AllowedPool(nil), r.Pools...),
		tags:                         append([]string(nil), r.Tags...),
		countries:                    append([]string(nil), r.Countries...),
		regions:                      append([]string(nil), r.Regions...),
		ipTypes:                      append([]string(nil), r.IPTypes...),
		ingressModes:                 append([]string(nil), r.IngressModes...),
		supportedFingerprintProfiles: append([]string(nil), r.SupportedFingerprintProfiles...),
		maxConcurrency:               r.MaxConcurrency,
		health:                       strawpb.WorkerHealth(r.Health),
		hasHeartbeat:                 r.HasHeartbeat,
		activeRequests:               r.ActiveRequests,
		availableCap:                 r.AvailableCap,
		draining:                     r.Draining,
		registeredAt:                 time.Unix(r.RegisteredUnix, r.RegisteredNano),
		lastHeartbeat:                time.Unix(r.HeartbeatUnix, r.HeartbeatNano),
	}
}

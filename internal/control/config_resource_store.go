package control

import (
	"context"
	"sort"
	"sync"

	"github.com/beremaran/straw/v2/internal/config"
)

// This file defines the store interfaces the config admin API handlers
// (config_admin_handlers.go) depend on, plus process-local implementations
// used by handler unit tests. PostgresConfigStore (postgres_config_store.go,
// postgres_config_list_store.go) implements all three interfaces against
// Postgres; the running binary wires that, never these in-memory doubles
// (docs/tasks/p0/20).

// RoutingRuleStore persists tenant-scoped routing rules with a client-supplied
// stable ID and optimistic concurrency on each rule's own config_version.
type RoutingRuleStore interface {
	ListRoutingRules(ctx context.Context, tenantID string, limit, offset int) ([]RoutingRuleRecord, error)
	GetRoutingRule(ctx context.Context, tenantID, id string) (RoutingRuleRecord, error)
	UpsertRoutingRule(ctx context.Context, tenantID string, rule config.RoutingRule, expectedVersion uint64, actor ConfigActor) (RoutingRuleRecord, uint64, error)
	DeleteRoutingRule(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error)
}

// ExecutorPoolStore persists tenant-scoped executor pools with a
// client-supplied stable ID and optimistic concurrency on each pool's own
// config_version (docs/tasks/p0/30).
type ExecutorPoolStore interface {
	ListExecutorPools(ctx context.Context, tenantID string, limit, offset int) ([]ExecutorPoolRecord, error)
	GetExecutorPool(ctx context.Context, tenantID, id string) (ExecutorPoolRecord, error)
	UpsertExecutorPool(ctx context.Context, tenantID string, pool config.ExecutorPool, expectedVersion uint64, actor ConfigActor) (ExecutorPoolRecord, uint64, error)
	DeleteExecutorPool(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error)
}

// DenyRuleStore persists tenant-scoped deny/allow rules, server-generating IDs.
type DenyRuleStore interface {
	ListDenyRules(ctx context.Context, tenantID string, limit, offset int) ([]DenyRuleRecord, error)
	GetDenyRule(ctx context.Context, tenantID, id string) (DenyRuleRecord, error)
	UpsertDenyRule(ctx context.Context, tenantID string, rule config.DenyRule, expectedVersion uint64, actor ConfigActor) (DenyRuleRecord, uint64, error)
	DeleteDenyRule(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error)
}

// InjectionPolicyStore persists tenant-scoped header-injection policies,
// server-generating IDs.
type InjectionPolicyStore interface {
	ListInjectionPolicies(ctx context.Context, tenantID string, limit, offset int) ([]InjectionPolicyRecord, error)
	GetInjectionPolicy(ctx context.Context, tenantID, id string) (InjectionPolicyRecord, error)
	UpsertInjectionPolicy(ctx context.Context, tenantID string, pol config.InjectionPolicy, expectedVersion uint64, actor ConfigActor) (InjectionPolicyRecord, uint64, error)
	DeleteInjectionPolicy(ctx context.Context, tenantID, id string, actor ConfigActor) (uint64, error)
}

// FingerprintProfileStore lists the fingerprint profiles visible to a tenant.
// P0 has no write path (docs/planning/26): profiles are seeded built-ins only.
type FingerprintProfileStore interface {
	ListFingerprintProfiles(ctx context.Context, tenantID string) ([]FingerprintProfileRecord, error)
}

// ---- in-memory test doubles ----

type inMemoryResource[T any] struct {
	value   T
	deleted bool
}

// InMemoryRoutingRuleStore is a handler-test double; production wires
// PostgresConfigStore.
type InMemoryRoutingRuleStore struct {
	mu   sync.Mutex
	byID map[string]map[string]*inMemoryResource[RoutingRuleRecord]
}

// NewInMemoryRoutingRuleStore builds an empty store.
func NewInMemoryRoutingRuleStore() *InMemoryRoutingRuleStore {
	return &InMemoryRoutingRuleStore{byID: make(map[string]map[string]*inMemoryResource[RoutingRuleRecord])}
}

// ListRoutingRules implements RoutingRuleStore.
func (s *InMemoryRoutingRuleStore) ListRoutingRules(_ context.Context, tenantID string, limit, offset int) ([]RoutingRuleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []RoutingRuleRecord

	for _, entry := range s.byID[tenantID] {
		if !entry.deleted {
			out = append(out, entry.value)
		}
	}

	sortRoutingRules(out)

	return paginate(out, limit, offset), nil
}

// GetRoutingRule implements RoutingRuleStore.
func (s *InMemoryRoutingRuleStore) GetRoutingRule(_ context.Context, tenantID, id string) (RoutingRuleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return RoutingRuleRecord{}, ErrConfigResourceNotFound
	}

	return entry.value, nil
}

// UpsertRoutingRule implements RoutingRuleStore.
func (s *InMemoryRoutingRuleStore) UpsertRoutingRule(_ context.Context, tenantID string, rule config.RoutingRule, expectedVersion uint64, _ ConfigActor) (RoutingRuleRecord, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.byID[tenantID] == nil {
		s.byID[tenantID] = make(map[string]*inMemoryResource[RoutingRuleRecord])
	}

	current := currentVersionOf(s.byID[tenantID][rule.ID])
	if current != expectedVersion {
		return RoutingRuleRecord{}, 0, ErrConfigResourceVersionConflict
	}

	record := RoutingRuleRecord{RoutingRule: rule, TenantID: tenantID, ConfigVersion: expectedVersion + 1}
	if existing := s.byID[tenantID][rule.ID]; existing != nil {
		record.CreatedAt = existing.value.CreatedAt
	}

	s.byID[tenantID][rule.ID] = &inMemoryResource[RoutingRuleRecord]{value: record}

	return record, expectedVersion + 1, nil
}

// DeleteRoutingRule implements RoutingRuleStore.
func (s *InMemoryRoutingRuleStore) DeleteRoutingRule(_ context.Context, tenantID, id string, _ ConfigActor) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return 0, ErrConfigResourceNotFound
	}

	entry.deleted = true

	return entry.value.ConfigVersion, nil
}

func sortRoutingRules(rules []RoutingRuleRecord) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].CreatedAt.Equal(rules[j].CreatedAt) {
			return rules[i].ID < rules[j].ID
		}

		return rules[i].CreatedAt.After(rules[j].CreatedAt)
	})
}

// InMemoryExecutorPoolStore is a handler-test double; production wires
// PostgresConfigStore.
type InMemoryExecutorPoolStore struct {
	mu   sync.Mutex
	byID map[string]map[string]*inMemoryResource[ExecutorPoolRecord]
}

// NewInMemoryExecutorPoolStore builds an empty store.
func NewInMemoryExecutorPoolStore() *InMemoryExecutorPoolStore {
	return &InMemoryExecutorPoolStore{byID: make(map[string]map[string]*inMemoryResource[ExecutorPoolRecord])}
}

// ListExecutorPools implements ExecutorPoolStore.
func (s *InMemoryExecutorPoolStore) ListExecutorPools(_ context.Context, tenantID string, limit, offset int) ([]ExecutorPoolRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []ExecutorPoolRecord

	for _, entry := range s.byID[tenantID] {
		if !entry.deleted {
			out = append(out, entry.value)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}

		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	return paginate(out, limit, offset), nil
}

// GetExecutorPool implements ExecutorPoolStore.
func (s *InMemoryExecutorPoolStore) GetExecutorPool(_ context.Context, tenantID, id string) (ExecutorPoolRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return ExecutorPoolRecord{}, ErrConfigResourceNotFound
	}

	return entry.value, nil
}

// UpsertExecutorPool implements ExecutorPoolStore.
func (s *InMemoryExecutorPoolStore) UpsertExecutorPool(_ context.Context, tenantID string, pool config.ExecutorPool, expectedVersion uint64, _ ConfigActor) (ExecutorPoolRecord, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.byID[tenantID] == nil {
		s.byID[tenantID] = make(map[string]*inMemoryResource[ExecutorPoolRecord])
	}

	current := currentVersionOf(s.byID[tenantID][pool.ID])
	if current != expectedVersion {
		return ExecutorPoolRecord{}, 0, ErrConfigResourceVersionConflict
	}

	record := ExecutorPoolRecord{ExecutorPool: pool, TenantID: tenantID, ConfigVersion: expectedVersion + 1}
	if existing := s.byID[tenantID][pool.ID]; existing != nil {
		record.CreatedAt = existing.value.CreatedAt
	}

	s.byID[tenantID][pool.ID] = &inMemoryResource[ExecutorPoolRecord]{value: record}

	return record, expectedVersion + 1, nil
}

// DeleteExecutorPool implements ExecutorPoolStore.
func (s *InMemoryExecutorPoolStore) DeleteExecutorPool(_ context.Context, tenantID, id string, _ ConfigActor) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return 0, ErrConfigResourceNotFound
	}

	entry.deleted = true

	return entry.value.ConfigVersion, nil
}

// InMemoryDenyRuleStore is a handler-test double; production wires
// PostgresConfigStore.
type InMemoryDenyRuleStore struct {
	mu   sync.Mutex
	byID map[string]map[string]*inMemoryResource[DenyRuleRecord]
}

// NewInMemoryDenyRuleStore builds an empty store.
func NewInMemoryDenyRuleStore() *InMemoryDenyRuleStore {
	return &InMemoryDenyRuleStore{byID: make(map[string]map[string]*inMemoryResource[DenyRuleRecord])}
}

// ListDenyRules implements DenyRuleStore.
func (s *InMemoryDenyRuleStore) ListDenyRules(_ context.Context, tenantID string, limit, offset int) ([]DenyRuleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []DenyRuleRecord

	for _, entry := range s.byID[tenantID] {
		if !entry.deleted {
			out = append(out, entry.value)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}

		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	return paginate(out, limit, offset), nil
}

// GetDenyRule implements DenyRuleStore.
func (s *InMemoryDenyRuleStore) GetDenyRule(_ context.Context, tenantID, id string) (DenyRuleRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return DenyRuleRecord{}, ErrConfigResourceNotFound
	}

	return entry.value, nil
}

// UpsertDenyRule implements DenyRuleStore.
func (s *InMemoryDenyRuleStore) UpsertDenyRule(_ context.Context, tenantID string, rule config.DenyRule, expectedVersion uint64, _ ConfigActor) (DenyRuleRecord, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.byID[tenantID] == nil {
		s.byID[tenantID] = make(map[string]*inMemoryResource[DenyRuleRecord])
	}

	current := currentVersionOf(s.byID[tenantID][rule.ID])
	if current != expectedVersion {
		return DenyRuleRecord{}, 0, ErrConfigResourceVersionConflict
	}

	record := DenyRuleRecord{DenyRule: rule, TenantID: tenantID, ConfigVersion: expectedVersion + 1}
	if existing := s.byID[tenantID][rule.ID]; existing != nil {
		record.CreatedAt = existing.value.CreatedAt
	}

	s.byID[tenantID][rule.ID] = &inMemoryResource[DenyRuleRecord]{value: record}

	return record, expectedVersion + 1, nil
}

// DeleteDenyRule implements DenyRuleStore.
func (s *InMemoryDenyRuleStore) DeleteDenyRule(_ context.Context, tenantID, id string, _ ConfigActor) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return 0, ErrConfigResourceNotFound
	}

	entry.deleted = true

	return entry.value.ConfigVersion, nil
}

// InMemoryInjectionPolicyStore is a handler-test double; production wires
// PostgresConfigStore.
type InMemoryInjectionPolicyStore struct {
	mu   sync.Mutex
	byID map[string]map[string]*inMemoryResource[InjectionPolicyRecord]
}

// NewInMemoryInjectionPolicyStore builds an empty store.
func NewInMemoryInjectionPolicyStore() *InMemoryInjectionPolicyStore {
	return &InMemoryInjectionPolicyStore{byID: make(map[string]map[string]*inMemoryResource[InjectionPolicyRecord])}
}

// ListInjectionPolicies implements InjectionPolicyStore.
func (s *InMemoryInjectionPolicyStore) ListInjectionPolicies(_ context.Context, tenantID string, limit, offset int) ([]InjectionPolicyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var out []InjectionPolicyRecord

	for _, entry := range s.byID[tenantID] {
		if !entry.deleted {
			out = append(out, entry.value)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}

		return out[i].CreatedAt.After(out[j].CreatedAt)
	})

	return paginate(out, limit, offset), nil
}

// GetInjectionPolicy implements InjectionPolicyStore.
func (s *InMemoryInjectionPolicyStore) GetInjectionPolicy(_ context.Context, tenantID, id string) (InjectionPolicyRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return InjectionPolicyRecord{}, ErrConfigResourceNotFound
	}

	return entry.value, nil
}

// UpsertInjectionPolicy implements InjectionPolicyStore.
func (s *InMemoryInjectionPolicyStore) UpsertInjectionPolicy(_ context.Context, tenantID string, pol config.InjectionPolicy, expectedVersion uint64, _ ConfigActor) (InjectionPolicyRecord, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(pol.Operations) > maxInjectionOperations {
		return InjectionPolicyRecord{}, 0, errInjectionPolicyTooLarge
	}

	if s.byID[tenantID] == nil {
		s.byID[tenantID] = make(map[string]*inMemoryResource[InjectionPolicyRecord])
	}

	current := currentVersionOf(s.byID[tenantID][pol.ID])
	if current != expectedVersion {
		return InjectionPolicyRecord{}, 0, ErrConfigResourceVersionConflict
	}

	record := InjectionPolicyRecord{InjectionPolicy: pol, TenantID: tenantID, ConfigVersion: expectedVersion + 1}
	if existing := s.byID[tenantID][pol.ID]; existing != nil {
		record.CreatedAt = existing.value.CreatedAt
	}

	s.byID[tenantID][pol.ID] = &inMemoryResource[InjectionPolicyRecord]{value: record}

	return record, expectedVersion + 1, nil
}

// DeleteInjectionPolicy implements InjectionPolicyStore.
func (s *InMemoryInjectionPolicyStore) DeleteInjectionPolicy(_ context.Context, tenantID, id string, _ ConfigActor) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.byID[tenantID][id]
	if !ok || entry.deleted {
		return 0, ErrConfigResourceNotFound
	}

	entry.deleted = true

	return entry.value.ConfigVersion, nil
}

// InMemoryFingerprintProfileStore is a handler-test double seeded with the
// same built-in profile names Postgres seeds via migration 0002.
type InMemoryFingerprintProfileStore struct {
	profiles []FingerprintProfileRecord
}

// fingerprintProfileScopeGlobal marks a built-in profile visible to every
// tenant (docs/planning/21: "P0 rows are seeded built-ins only").
const fingerprintProfileScopeGlobal = "global"

// NewInMemoryFingerprintProfileStore builds a store seeded with P0's built-in
// global profiles.
func NewInMemoryFingerprintProfileStore() *InMemoryFingerprintProfileStore {
	names := []string{defaultFingerprintProfileName, "chrome_120", "firefox_121", "safari_17"}
	profiles := make([]FingerprintProfileRecord, 0, len(names))

	for _, name := range names {
		profiles = append(profiles, FingerprintProfileRecord{
			FingerprintProfile: config.FingerprintProfile{
				Name: name, ScopeType: fingerprintProfileScopeGlobal, SupportedByWorker: true, Enabled: true,
			},
			ConfigVersion: 1,
		})
	}

	return &InMemoryFingerprintProfileStore{profiles: profiles}
}

// ListFingerprintProfiles implements FingerprintProfileStore.
func (s *InMemoryFingerprintProfileStore) ListFingerprintProfiles(_ context.Context, _ string) ([]FingerprintProfileRecord, error) {
	return s.profiles, nil
}

func currentVersionOf[T any](entry *inMemoryResource[T]) uint64 {
	if entry == nil || entry.deleted {
		return 0
	}

	switch v := any(entry.value).(type) {
	case RoutingRuleRecord:
		return v.ConfigVersion
	case ExecutorPoolRecord:
		return v.ConfigVersion
	case DenyRuleRecord:
		return v.ConfigVersion
	case InjectionPolicyRecord:
		return v.ConfigVersion
	default:
		return 0
	}
}

func paginate[T any](items []T, limit, offset int) []T {
	limit = clampConfigListLimit(limit)

	if offset < 0 {
		offset = 0
	}

	if offset >= len(items) {
		return nil
	}

	end := min(offset+limit, len(items))

	return items[offset:end]
}

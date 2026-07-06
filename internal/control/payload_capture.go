package control

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// CaptureDecision is a tenant-authorized payload capture level.
type CaptureDecision string

// Capture decisions match docs/planning/19-payload-capture-p2.md.
const (
	CaptureDecisionNone          CaptureDecision = "none"
	CaptureDecisionMetadataOnly  CaptureDecision = "metadata_only"
	CaptureDecisionHeaders       CaptureDecision = "headers"
	CaptureDecisionBodyTruncated CaptureDecision = "body_truncated"
	CaptureDecisionBodyFull      CaptureDecision = "body_full"
)

// PayloadCapturePolicy is one tenant's payload-capture authorization policy.
type PayloadCapturePolicy struct {
	TenantID         string
	Enabled          bool
	AllowedDecisions []CaptureDecision
	ConfigVersion    uint64
}

// ErrPayloadCaptureVersionConflict is returned on optimistic concurrency failure.
var ErrPayloadCaptureVersionConflict = errors.New("payload capture config version conflict")

// PayloadCapturePolicyStore persists per-tenant payload-capture policy.
type PayloadCapturePolicyStore interface {
	Get(ctx context.Context, tenantID string) (PayloadCapturePolicy, error)
	Put(ctx context.Context, policy PayloadCapturePolicy, expectedVersion uint64) (PayloadCapturePolicy, error)
}

// InMemoryPayloadCapturePolicyStore stores payload-capture policy for tests.
type InMemoryPayloadCapturePolicyStore struct {
	mu    sync.Mutex
	byTid map[string]PayloadCapturePolicy
}

// NewInMemoryPayloadCapturePolicyStore creates an empty test policy store.
func NewInMemoryPayloadCapturePolicyStore() *InMemoryPayloadCapturePolicyStore {
	return &InMemoryPayloadCapturePolicyStore{byTid: make(map[string]PayloadCapturePolicy)}
}

func defaultPayloadCapturePolicy(tenantID string) PayloadCapturePolicy {
	return PayloadCapturePolicy{TenantID: tenantID, AllowedDecisions: []CaptureDecision{CaptureDecisionNone}}
}

// Get returns a tenant policy, defaulting to disabled and none-only.
func (s *InMemoryPayloadCapturePolicyStore) Get(_ context.Context, tenantID string) (PayloadCapturePolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	policy, ok := s.byTid[tenantID]
	if !ok {
		return defaultPayloadCapturePolicy(tenantID), nil
	}

	return policy, nil
}

// Put updates a tenant policy under optimistic concurrency.
func (s *InMemoryPayloadCapturePolicyStore) Put(_ context.Context, policy PayloadCapturePolicy, expectedVersion uint64) (PayloadCapturePolicy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current := uint64(0)
	if existing, ok := s.byTid[policy.TenantID]; ok {
		current = existing.ConfigVersion
	}

	if current != expectedVersion {
		return PayloadCapturePolicy{}, ErrPayloadCaptureVersionConflict
	}

	policy.ConfigVersion = current + 1
	s.byTid[policy.TenantID] = policy

	return policy, nil
}

func payloadCaptureAllows(hint string, policy PayloadCapturePolicy) bool {
	decision := CaptureDecision(hint)
	if decision == "" {
		decision = CaptureDecisionNone
	}

	if decision == CaptureDecisionNone {
		return true
	}

	if !policy.Enabled {
		return false
	}

	return slices.Contains(policy.AllowedDecisions, decision)
}

func validCaptureDecision(decision CaptureDecision) bool {
	switch decision {
	case CaptureDecisionNone, CaptureDecisionMetadataOnly, CaptureDecisionHeaders, CaptureDecisionBodyTruncated, CaptureDecisionBodyFull:
		return true
	default:
		return false
	}
}

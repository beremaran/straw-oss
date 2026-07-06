package control

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"
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

// Default capture limits.
const (
	DefaultMaxFullBytes      = 1024 * 1024 // 1MB
	DefaultMaxTruncatedBytes = 4096        // 4KB
)

// CaptureOptions configures payload capture bounds and behavior.
type CaptureOptions struct {
	MaxFullBytes      int
	MaxTruncatedBytes int
	AllowCompressed   bool
}

// CaptureResult represents the output of the payload capture engine.
type CaptureResult struct {
	CapturedAt      time.Time
	RequestHeaders  string
	ResponseHeaders string
	RequestBody     []byte
	ResponseBody    []byte
	RedactedFields  []string
	Truncated       bool
	Decision        CaptureDecision
}

// CapturedHeader represents a captured header in storage format.
type CapturedHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CapturePayload executes the payload capture rules on request and response.
// It returns a non-mutated copy of headers and bodies adhering to the capture decision.
func CapturePayload(
	decision CaptureDecision,
	reqHeaders []HeaderPair,
	reqBody []byte,
	respHeaders []HeaderPair,
	respBody []byte,
	opts CaptureOptions,
) CaptureResult {
	res := CaptureResult{
		CapturedAt: time.Now().UTC(),
		Decision:   decision,
	}

	if decision == CaptureDecisionNone || decision == CaptureDecisionMetadataOnly {
		return res
	}

	var (
		reqRedacted  []string
		respRedacted []string
	)

	res.RequestHeaders, reqRedacted = redactAndSerializeHeaders(reqHeaders)
	res.ResponseHeaders, respRedacted = redactAndSerializeHeaders(respHeaders)
	res.RedactedFields = mergeUnique(res.RedactedFields, reqRedacted, respRedacted)

	if decision == CaptureDecisionHeaders {
		return res
	}

	limit := getCaptureLimit(decision, opts)

	var (
		reqTruncated  bool
		respTruncated bool
	)

	res.RequestBody, reqTruncated = captureBody(reqBody, reqHeaders, limit, opts.AllowCompressed)
	res.ResponseBody, respTruncated = captureBody(respBody, respHeaders, limit, opts.AllowCompressed)
	res.Truncated = reqTruncated || respTruncated

	return res
}

func getCaptureLimit(decision CaptureDecision, opts CaptureOptions) int {
	limit := opts.MaxFullBytes

	if decision == CaptureDecisionBodyTruncated {
		limit = opts.MaxTruncatedBytes
	}

	if limit <= 0 {
		limit = DefaultMaxFullBytes

		if decision == CaptureDecisionBodyTruncated {
			limit = DefaultMaxTruncatedBytes
		}
	}

	return limit
}

func mergeUnique(dest []string, src ...[]string) []string {
	for _, slice := range src {
		for _, item := range slice {
			if !slices.Contains(dest, item) {
				dest = append(dest, item)
			}
		}
	}

	return dest
}

func captureBody(body []byte, headers []HeaderPair, limit int, allowCompressed bool) ([]byte, bool) {
	if len(body) == 0 {
		return nil, false
	}

	if isCompressed(headers) && !allowCompressed {
		return nil, false
	}

	if len(body) > limit {
		captured := make([]byte, limit)

		copy(captured, body[:limit])

		return captured, true
	}

	captured := make([]byte, len(body))

	copy(captured, body)

	return captured, false
}

func redactAndSerializeHeaders(headers []HeaderPair) (string, []string) {
	if len(headers) == 0 {
		return "[]", nil
	}

	var redactedFields []string

	captured := make([]CapturedHeader, 0, len(headers))

	for _, h := range headers {
		name := h.Name
		valBytes, err := base64.StdEncoding.DecodeString(h.Value)

		var val string

		if err != nil {
			val = h.Value
		} else {
			val = string(valBytes)
		}

		if isSensitiveHeaderName(name) {
			val = requestMetadataRedacted

			if !slices.Contains(redactedFields, name) {
				redactedFields = append(redactedFields, name)
			}
		}

		captured = append(captured, CapturedHeader{
			Name:  name,
			Value: val,
		})
	}

	b, err := json.Marshal(captured)
	if err != nil {
		return "[]", redactedFields
	}

	return string(b), redactedFields
}

func isCompressed(headers []HeaderPair) bool {
	for _, h := range headers {
		if strings.EqualFold(h.Name, "content-encoding") {
			valBytes, err := base64.StdEncoding.DecodeString(h.Value)

			var val string

			if err != nil {
				val = h.Value
			} else {
				val = string(valBytes)
			}

			val = strings.TrimSpace(strings.ToLower(val))

			if val != "" && val != "identity" {
				return true
			}
		}
	}

	return false
}

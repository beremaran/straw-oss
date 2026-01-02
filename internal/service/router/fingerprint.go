package router

import (
	"crypto/rand"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/kwilabs/straw-proxy-server/internal/domain"
)

// FingerprintManager handles the logic for selecting a fingerprint based on routing rules.
type FingerprintManager struct {
	// rrCounters stores the round-robin counters for each rule ID.
	rrCounters sync.Map
}

// NewFingerprintManager creates a new FingerprintManager.
func NewFingerprintManager() *FingerprintManager {
	return &FingerprintManager{}
}

// SelectFingerprint returns the selected fingerprint ID for the given rule.
func (fm *FingerprintManager) SelectFingerprint(rule *domain.RoutingRule) string {
	if rule == nil {
		return ""
	}

	// 1. Check for A/B Test configuration
	if rule.FingerprintABTest != nil && len(rule.FingerprintABTest.Variants) > 0 {
		return fm.selectFromAB(rule.ID, rule.FingerprintABTest)
	}

	// 2. Return static preset if no A/B test
	return rule.FingerprintPreset
}

// selectFromAB selects a variant based on the configured strategy.
func (fm *FingerprintManager) selectFromAB(ruleID string, config *domain.ABConfig) string {
	if len(config.Variants) == 0 {
		return ""
	}

	var selected string
	switch config.Strategy {
	case "weighted":
		selected = fm.selectWeighted(config.Variants)
	case "round_robin":
		selected = fm.selectRoundRobin(ruleID, config.Variants)
	default: // "random" or unknown
		selected = fm.selectRandom(config.Variants)
	}

	// Track usage for A/B analysis
	// In production, this should be a metric counter or structured audit log
	// log.Printf("AB_TEST: rule=%s strategy=%s variant=%s", ruleID, config.Strategy, selected)

	return selected
}

// selectRandom selects a variant uniformly at random.
func (fm *FingerprintManager) selectRandom(variants []domain.ABVariant) string {
	idx := safeRandInt(len(variants))
	return variants[idx].Fingerprint
}

// selectWeighted selects a variant based on weight.
func (fm *FingerprintManager) selectWeighted(variants []domain.ABVariant) string {
	totalWeight := 0
	for _, v := range variants {
		totalWeight += v.Weight
	}

	if totalWeight <= 0 {
		// Fallback to random if weights are invalid
		return fm.selectRandom(variants)
	}

	r := safeRandInt(totalWeight)
	current := 0
	for _, v := range variants {
		current += v.Weight
		if r < current {
			return v.Fingerprint
		}
	}

	// Should not be reached if logic is correct, fallback to last
	return variants[len(variants)-1].Fingerprint
}

// selectRoundRobin selects a variant sequentially.
// Note: State is in-memory and resets on restart.
func (fm *FingerprintManager) selectRoundRobin(ruleID string, variants []domain.ABVariant) string {
	// Get or create counter for this rule
	val, loaded := fm.rrCounters.LoadOrStore(ruleID, new(uint64))
	counter := val.(*uint64)

	// Increment and get index
	// We use atomic add before using it to ensure unique values for concurrent calls,
	// but simplified to just get next value.
	// 0-indexed: (val % len)
	// We want to start from 0 if it's new.
	// If loaded, we want the next one.
	// Actually, just atomic.AddUint64(counter, 1) return new value.
	// But we want to start at 0. So let's do atomic.AddUint64 then subtract 1.
	// Or just use the value directly for mod.

	// If it was just loaded (new), it is 0.
	// We can't know if another goroutine incremented it between LoadOrStore and now
	// without the pointer. Since we have the pointer to the heap allocated uint64,
	// atomic operations are safe.

	var nextVal uint64
	if !loaded {
		// Initialize to 0 is implicit with new(uint64), use it as is?
		// But we want the FIRST call to return index 0.
		// If we do Add(1), we get 1. (1-1)%len = 0.
		nextVal = atomic.AddUint64(counter, 1)
	} else {
		nextVal = atomic.AddUint64(counter, 1)
	}

	idx := int((nextVal - 1) % uint64(len(variants)))
	return variants[idx].Fingerprint
}

// safeRandInt returns a secure random integer in [0, max).
// If error occurs (rare), it uses a pseudo-random fallback (not implemented here strictly, assumes crypto/rand works, panics otherwise or returns 0).
func safeRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		// Fallback or panic? For this system, let's return 0 to be safe from panic,
		// but this indicates a system-level issue.
		return 0
	}
	return int(n.Int64())
}

package router

import (
	"crypto/rand"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/beremaran/straw/internal/domain"
)

const (
	strategyWeighted   = "weighted"
	strategyRoundRobin = "round_robin"
)

// FingerprintManager selects fingerprints for routing rules using various strategies.
type FingerprintManager struct {
	rrCounters sync.Map
}

// NewFingerprintManager creates a new FingerprintManager.
func NewFingerprintManager() *FingerprintManager {
	return &FingerprintManager{}
}

// SelectFingerprint selects a fingerprint for the given routing rule.
func (fm *FingerprintManager) SelectFingerprint(rule *domain.RoutingRule) string {
	if rule == nil {
		return ""
	}

	if rule.FingerprintABTest != nil && len(rule.FingerprintABTest.Variants) > 0 {
		return fm.selectFromAB(rule.ID, rule.FingerprintABTest)
	}

	return rule.FingerprintPreset
}

func (fm *FingerprintManager) selectFromAB(ruleID string, config *domain.ABConfig) string {
	if len(config.Variants) == 0 {
		return ""
	}

	var selected string

	switch config.Strategy {
	case strategyWeighted:
		selected = fm.selectWeighted(config.Variants)
	case strategyRoundRobin:
		selected = fm.selectRoundRobin(ruleID, config.Variants)
	default:
		selected = fm.selectRandom(config.Variants)
	}

	return selected
}

func (fm *FingerprintManager) selectRandom(variants []domain.ABVariant) string {
	idx := safeRandInt(len(variants))

	return variants[idx].Fingerprint
}

func (fm *FingerprintManager) selectWeighted(variants []domain.ABVariant) string {
	totalWeight := 0
	for _, v := range variants {
		totalWeight += v.Weight
	}

	if totalWeight <= 0 {
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

	return variants[len(variants)-1].Fingerprint
}

func (fm *FingerprintManager) selectRoundRobin(ruleID string, variants []domain.ABVariant) string {
	val, loaded := fm.rrCounters.LoadOrStore(ruleID, new(int64))
	counter := val.(*int64)

	var nextVal int64

	nextVal = atomic.AddInt64(counter, 1)
	if !loaded {
		nextVal = 1
	}

	idx := int(nextVal-1) % len(variants)

	return variants[idx].Fingerprint
}

func safeRandInt(limit int) int {
	if limit <= 0 {
		return 0
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0
	}

	return int(n.Int64())
}

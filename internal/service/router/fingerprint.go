package router

import (
	"crypto/rand"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/beremaran/straw/internal/domain"
)

type FingerprintManager struct {
	rrCounters sync.Map
}

func NewFingerprintManager() *FingerprintManager {
	return &FingerprintManager{}
}

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
	case "weighted":
		selected = fm.selectWeighted(config.Variants)
	case "round_robin":
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
	val, loaded := fm.rrCounters.LoadOrStore(ruleID, new(uint64))
	counter := val.(*uint64)

	var nextVal uint64
	if !loaded {
		nextVal = atomic.AddUint64(counter, 1)
	} else {
		nextVal = atomic.AddUint64(counter, 1)
	}

	idx := int((nextVal - 1) % uint64(len(variants)))

	return variants[idx].Fingerprint
}

func safeRandInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}

	return int(n.Int64())
}

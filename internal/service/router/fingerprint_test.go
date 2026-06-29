package router

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/beremaran/straw/internal/domain"
)

const (
	testStrategyWeighted   = "weighted"
	testStrategyRoundRobin = "round_robin"
	testFingerprintRuleID  = "rule-1"
)

func TestFingerprintManager_SelectFingerprint(t *testing.T) {
	fm := NewFingerprintManager()

	t.Run("Static Preset", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID:                testFingerprintRuleID,
			FingerprintPreset: "chrome-100",
		}
		fp := fm.SelectFingerprint(rule)
		assert.Equal(t, "chrome-100", fp)
	})

	t.Run("Nil Rule", func(t *testing.T) {
		fp := fm.SelectFingerprint(nil)
		assert.Empty(t, fp)
	})

	t.Run("AB Test - Random", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID: "rule-random",
			FingerprintABTest: &domain.ABConfig{
				Strategy: "random",
				Variants: []domain.ABVariant{
					{Fingerprint: "fp1", Weight: 0},
					{Fingerprint: "fp2", Weight: 0},
				},
			},
		}

		counts := make(map[string]int)
		for range 100 {
			fp := fm.SelectFingerprint(rule)
			counts[fp]++
		}

		assert.Contains(t, counts, "fp1")
		assert.Contains(t, counts, "fp2")
	})

	t.Run("AB Test - Weighted", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID: "rule-weighted",
			FingerprintABTest: &domain.ABConfig{
				Strategy: testStrategyWeighted,
				Variants: []domain.ABVariant{
					{Fingerprint: "heavy", Weight: 90},
					{Fingerprint: "light", Weight: 10},
				},
			},
		}

		heavyCount := 0
		total := 1000
		for range total {
			fp := fm.SelectFingerprint(rule)
			if fp == "heavy" {
				heavyCount++
			}
		}

		assert.Greater(t, heavyCount, 800)
	})

	t.Run("AB Test - Round Robin", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID: "rule-rr",
			FingerprintABTest: &domain.ABConfig{
				Strategy: testStrategyRoundRobin,
				Variants: []domain.ABVariant{
					{Fingerprint: "A"},
					{Fingerprint: "B"},
					{Fingerprint: "C"},
				},
			},
		}

		assert.Equal(t, "A", fm.SelectFingerprint(rule))
		assert.Equal(t, "B", fm.SelectFingerprint(rule))
		assert.Equal(t, "C", fm.SelectFingerprint(rule))
		assert.Equal(t, "A", fm.SelectFingerprint(rule))
	})

	t.Run("AB Test - Round Robin Concurrent", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID: "rule-rr-conc",
			FingerprintABTest: &domain.ABConfig{
				Strategy: testStrategyRoundRobin,
				Variants: []domain.ABVariant{
					{Fingerprint: "A"},
					{Fingerprint: "B"},
				},
			},
		}

		countA := 0
		countB := 0

		results := make(chan string, 100)

		for range 100 {
			go func() {
				results <- fm.SelectFingerprint(rule)
			}()
		}

		for range 100 {
			res := <-results
			if res == "A" {
				countA++
			} else {
				countB++
			}
		}

		assert.Equal(t, 50, countA)
		assert.Equal(t, 50, countB)
	})

	t.Run("Weighted Zero Total Weight", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID: "rule-weighted-zero",
			FingerprintABTest: &domain.ABConfig{
				Strategy: testStrategyWeighted,
				Variants: []domain.ABVariant{
					{Fingerprint: "A", Weight: 0},
					{Fingerprint: "B", Weight: 0},
				},
			},
		}

		counts := make(map[string]int)
		for range 50 {
			fp := fm.SelectFingerprint(rule)
			counts[fp]++
		}
		assert.True(t, counts["A"] > 0 || counts["B"] > 0)
	})

	t.Run("Unknown Strategy Fallback", func(t *testing.T) {
		rule := &domain.RoutingRule{
			FingerprintABTest: &domain.ABConfig{
				Strategy: "invalid",
				Variants: []domain.ABVariant{
					{Fingerprint: "A"},
				},
			},
		}
		assert.Equal(t, "A", fm.SelectFingerprint(rule))
	})

	t.Run("Empty Variants", func(t *testing.T) {
		rule := &domain.RoutingRule{
			FingerprintABTest: &domain.ABConfig{
				Strategy: "random",
				Variants: []domain.ABVariant{},
			},
		}
		fp := fm.SelectFingerprint(rule)
		assert.Empty(t, fp)
	})
}

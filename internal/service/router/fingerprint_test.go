package router

import (
	"testing"

	"github.com/beremaran/straw/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestFingerprintManager_SelectFingerprint(t *testing.T) {
	fm := NewFingerprintManager()

	t.Run("Static Preset", func(t *testing.T) {
		rule := &domain.RoutingRule{
			ID:                "rule-1",
			FingerprintPreset: "chrome-100",
		}
		fp := fm.SelectFingerprint(rule)
		assert.Equal(t, "chrome-100", fp)
	})

	t.Run("Nil Rule", func(t *testing.T) {
		fp := fm.SelectFingerprint(nil)
		assert.Equal(t, "", fp)
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
		for i := 0; i < 100; i++ {
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
				Strategy: "weighted",
				Variants: []domain.ABVariant{
					{Fingerprint: "heavy", Weight: 90},
					{Fingerprint: "light", Weight: 10},
				},
			},
		}

		heavyCount := 0
		total := 1000
		for i := 0; i < total; i++ {
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
				Strategy: "round_robin",
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
				Strategy: "round_robin",
				Variants: []domain.ABVariant{
					{Fingerprint: "A"},
					{Fingerprint: "B"},
				},
			},
		}

		countA := 0
		countB := 0

		results := make(chan string, 100)

		for i := 0; i < 100; i++ {
			go func() {
				results <- fm.SelectFingerprint(rule)
			}()
		}

		for i := 0; i < 100; i++ {
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
				Strategy: "weighted",
				Variants: []domain.ABVariant{
					{Fingerprint: "A", Weight: 0},
					{Fingerprint: "B", Weight: 0},
				},
			},
		}

		counts := make(map[string]int)
		for i := 0; i < 50; i++ {
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
		assert.Equal(t, "", fp)
	})
}

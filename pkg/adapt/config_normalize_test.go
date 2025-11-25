// Package adapt pins coverage for normalization helpers so defaults and clamps
// stay documented alongside the configuration layer.
//
//nolint:testpackage,godoclint // Tests rely on private helpers for coverage.
package adapt

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestEnsureDurationUsesFallback(t *testing.T) {
	t.Parallel()

	fallback := 10 * time.Second

	if got := ensureDuration(0, fallback); got != fallback {
		t.Fatalf("expected fallback duration, got %v", got)
	}

	if got := ensureDuration(-time.Minute, fallback); got != fallback {
		t.Fatalf("expected fallback for negative duration, got %v", got)
	}
}

func TestEnsureDurationKeepsPositive(t *testing.T) {
	t.Parallel()

	expected := 5 * time.Second

	if got := ensureDuration(expected, time.Minute); got != expected {
		t.Fatalf("expected positive duration to be preserved, got %v", got)
	}
}

func TestEnsureFloatUsesFallback(t *testing.T) {
	t.Parallel()

	fallback := 0.75

	if got := ensureFloat(0, fallback); got != fallback {
		t.Fatalf("expected fallback float, got %f", got)
	}
}

func TestEnsureFloatKeepsNonZero(t *testing.T) {
	t.Parallel()

	expected := 0.42

	if got := ensureFloat(expected, 0.75); got != expected {
		t.Fatalf("expected non-zero float to be preserved, got %f", got)
	}
}

func TestEnsureFloatAllowZeroRespectsZero(t *testing.T) {
	t.Parallel()

	fallback := 0.75

	if got := ensureFloatAllowZero(0, fallback); got != 0 {
		t.Fatalf("expected zero to be preserved, got %f", got)
	}
}

func TestEnsureIntUsesFallbackForZero(t *testing.T) {
	t.Parallel()

	if got := ensureInt(0, 3); got != 3 {
		t.Fatalf("expected fallback for zero confirmations, got %d", got)
	}
}

func TestEnsureIntKeepsPositive(t *testing.T) {
	t.Parallel()

	if got := ensureInt(4, 2); got != 4 {
		t.Fatalf("expected positive confirmation count to be preserved, got %d", got)
	}
}

func TestEnsureIntKeepsNegative(t *testing.T) {
	t.Parallel()

	if got := ensureInt(-1, 2); got != -1 {
		t.Fatalf("expected negative confirmation count to be preserved for validation, got %d", got)
	}
}

func TestNormalizeConfigAdjustsSuppressResume(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressThreshold = 0.6
	cfg.SuppressResume = 0.9

	normalized, mode, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatalf("normalizeConfig returned error: %v", err)
	}

	if mode != defaultModeLabel {
		t.Fatalf("expected default mode label, got %q", mode)
	}

	expectedResume := cfg.SuppressThreshold * suppressResumeScale
	if math.Abs(normalized.SuppressResume-expectedResume) > 1e-6 {
		t.Fatalf(
			"expected suppress resume %.2f, got %.2f",
			expectedResume,
			normalized.SuppressResume,
		)
	}
}

func TestNormalizeConfigDefaultsRelaxedConfirmations(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.RelaxedConfirmations = 0

	normalized, _, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatalf("normalizeConfig returned error: %v", err)
	}

	if normalized.RelaxedConfirmations != defaultRelaxedConfirmations {
		t.Fatalf(
			"expected relaxed confirmations to fall back to default %d, got %d",
			defaultRelaxedConfirmations,
			normalized.RelaxedConfirmations,
		)
	}
}

func TestNormalizeConfigDisablesSuppressionWhenThresholdZero(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressThreshold = 0
	// Leave resume at a non-zero value to ensure normalization resets it.
	cfg.SuppressResume = 0.5

	normalized, _, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatalf("normalizeConfig returned error: %v", err)
	}

	if normalized.SuppressThreshold != 0 {
		t.Fatalf("expected suppress threshold to remain 0, got %.2f", normalized.SuppressThreshold)
	}

	if normalized.SuppressResume != 0 {
		t.Fatalf(
			"expected suppress resume to reset to 0 when suppression is disabled, got %.2f",
			normalized.SuppressResume,
		)
	}
}

func TestNormalizeConfigAdjustsRunnableResume(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressRunnableThreshold = 1.4
	cfg.SuppressRunnableResume = 1.5

	normalized, _, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatalf("normalizeConfig returned error: %v", err)
	}

	expected := cfg.SuppressRunnableThreshold * suppressResumeScale
	if math.Abs(normalized.SuppressRunnableResume-expected) > 1e-6 {
		t.Fatalf(
			"expected runnable resume %.2f, got %.2f",
			expected,
			normalized.SuppressRunnableResume,
		)
	}
}

func TestNormalizeConfigDisablesRunnableSuppressionWhenThresholdZero(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressRunnableThreshold = 0
	cfg.SuppressRunnableResume = 1

	normalized, _, err := normalizeConfig(cfg)
	if err != nil {
		t.Fatalf("normalizeConfig returned error: %v", err)
	}

	if normalized.SuppressRunnableResume != 0 {
		t.Fatalf(
			"expected runnable resume reset to 0 when disabled, got %.2f",
			normalized.SuppressRunnableResume,
		)
	}
}

func TestNormalizeConfigValidatesThresholds(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressThreshold = cfg.TargetStart - 0.1

	_, _, err := normalizeConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

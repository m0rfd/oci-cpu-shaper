// Package adapt uses this file to pin coverage for the configuration helpers so
// new defaults and validation rules stay documented in tests.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()

	err := ValidateConfig(cfg)
	if err != nil {
		t.Fatalf("ValidateConfig returned error for defaults: %v", err)
	}

	invalid := cfg
	invalid.SuppressThreshold = 0.20

	err = ValidateConfig(invalid)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

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

func TestStateStringUnknown(t *testing.T) {
	t.Parallel()

	if got := State(-1).String(); got != "unknown" {
		t.Fatalf("expected unknown state string, got %q", got)
	}
}

func TestClampEnforcesBounds(t *testing.T) {
	t.Parallel()

	if got := clamp(-0.5, 0, 1); got != 0 {
		t.Fatalf("expected clamp to return lower bound, got %f", got)
	}

	if got := clamp(1.5, 0, 1); got != 1 {
		t.Fatalf("expected clamp to return upper bound, got %f", got)
	}

	if got := clamp(0.5, 0, 1); got != 0.5 {
		t.Fatalf("expected clamp to preserve value within bounds, got %f", got)
	}
}

func TestValidateConfigAllowsDisabledSuppression(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressThreshold = 0
	cfg.SuppressResume = 0

	err := ValidateConfig(cfg)
	if err != nil {
		t.Fatalf("ValidateConfig returned error for disabled suppression: %v", err)
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

func TestNormalizeConfigValidatesThresholds(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressThreshold = cfg.TargetStart - 0.1

	_, _, err := normalizeConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestModeEnforcesTargets(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		mode      string
		enforcing bool
	}{
		{name: "dry-run disabled", mode: dryRunModeLabel, enforcing: false},
		{name: "empty defaults to enforcing", mode: "", enforcing: true},
		{name: "explicit enforce", mode: "enforce", enforcing: true},
		{name: "case insensitive", mode: " EnFoRcE ", enforcing: true},
		{name: "unknown modes enforce", mode: "observe", enforcing: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ModeEnforcesTargets(tc.mode); got != tc.enforcing {
				t.Fatalf("expected enforcing=%v for mode %q, got %v", tc.enforcing, tc.mode, got)
			}
		})
	}
}

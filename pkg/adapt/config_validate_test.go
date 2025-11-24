// Package adapt keeps validation coverage colocated with the helpers so new
// thresholds and guardrails remain documented in tests.
//
//nolint:testpackage,godoclint // Tests exercise private helpers for coverage notes.
package adapt

import (
	"errors"
	"testing"
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

func TestValidateConfigRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TargetMin = cfg.TargetMax

	err := ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for equal target bounds, got %v", err)
	}
}

func TestValidateConfigRejectsThresholdsOutsideBounds(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TargetStart = cfg.TargetMax + 0.1

	err := ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for out-of-range targetStart, got %v", err)
	}
}

func TestValidateConfigRejectsNonPositiveSteps(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.StepUp = -0.01

	err := ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for negative stepUp, got %v", err)
	}
}

func TestValidateConfigRejectsDescendingGoals(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.GoalLow = cfg.GoalHigh + 0.01

	err := ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for descending goals, got %v", err)
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

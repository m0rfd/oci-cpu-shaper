// Package adapt keeps validation coverage colocated with the helpers so new
// thresholds and guardrails remain documented in tests.
//
//nolint:testpackage,godoclint // Tests exercise private helpers for coverage notes.
package adapt

import (
	"errors"
	"strings"
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

func TestValidateConfigRejectsNonPositiveRelaxedConfirmations(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.RelaxedConfirmations = -1

	err := ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for non-positive relaxed confirmations, got %v", err)
	}
}

func TestValidateConfigRejectsExcessiveRelaxedConfirmations(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.RelaxedConfirmations = 101 // Above maximum of 100

	err := ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for excessive relaxed confirmations, got %v", err)
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

func TestValidateConfigRejectsInvalidRunnableSuppression(t *testing.T) {
	t.Parallel()

	thresholdErr := "controller.suppressRunnableThreshold (-1.00) must be zero or greater"
	resumeErr := "controller.suppressRunnableResume (2.00) must be less than " +
		"controller.suppressRunnableThreshold (1.00)"
	resumeEqualErr := "controller.suppressRunnableResume (1.00) must be less than " +
		"controller.suppressRunnableThreshold (1.00)"

	testCases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name: "negative runnable threshold",
			mutate: func(cfg *Config) {
				cfg.SuppressRunnableThreshold = -1
			},
			want: thresholdErr,
		},
		{
			name: "negative runnable resume handled after threshold",
			mutate: func(cfg *Config) {
				cfg.SuppressRunnableThreshold = -2
				cfg.SuppressRunnableResume = -1
			},
			want: "controller.suppressRunnableThreshold (-2.00) must be zero or greater",
		},
		{
			name: "resume exceeds threshold",
			mutate: func(cfg *Config) {
				cfg.SuppressRunnableThreshold = 1
				cfg.SuppressRunnableResume = 2
			},
			want: resumeErr,
		},
		{
			name: "resume equals threshold",
			mutate: func(cfg *Config) {
				cfg.SuppressRunnableThreshold = 1
				cfg.SuppressRunnableResume = 1
			},
			want: resumeEqualErr,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			testCase.mutate(&cfg)

			assertInvalidRunnableConfig(t, cfg, testCase.want)
		})
	}
}

func assertInvalidRunnableConfig(t *testing.T, cfg Config, wantMsg string) {
	t.Helper()

	err := ValidateConfig(cfg)
	if err == nil || !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}

	if !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("expected error containing %q, got %v", wantMsg, err)
	}
}

func TestValidateConfigAllowsDisabledRunnableSuppression(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressRunnableThreshold = 0
	cfg.SuppressRunnableResume = 0

	err := ValidateConfig(cfg)
	if err != nil {
		t.Fatalf("ValidateConfig returned error for disabled runnable suppression: %v", err)
	}
}

func TestValidateConfigRejectsInvalidSmoothing(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.SuppressSmoothingSamples = -1

	err := ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for negative smoothing, got %v", err)
	}

	cfg.SuppressSmoothingSamples = 101

	err = ValidateConfig(cfg)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig for excessive smoothing, got %v", err)
	}
}

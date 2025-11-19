package adapt

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Config defines controller thresholds.
type Config struct {
	ResourceID        string
	Mode              string
	TargetStart       float64
	TargetMin         float64
	TargetMax         float64
	StepUp            float64
	StepDown          float64
	FallbackTarget    float64
	GoalLow           float64
	GoalHigh          float64
	Interval          time.Duration
	RelaxedInterval   time.Duration
	RelaxedThreshold  float64
	SuppressThreshold float64
	SuppressResume    float64
}

const (
	defaultModeLabel       = "normal"
	dryRunModeLabel        = "dry-run"
	noopModeLabel          = "noop"
	defaultTargetStart     = 0.25
	defaultTargetMin       = 0.22
	defaultTargetMax       = 0.40
	defaultStepUp          = 0.02
	defaultStepDown        = 0.01
	defaultFallbackTarget  = 0.25
	defaultGoalLow         = 0.23
	defaultGoalHigh        = 0.30
	defaultRelaxedInterval = 6 * time.Hour
	defaultRelaxedThresh   = 0.28
	defaultSuppressThresh  = 0.85
	defaultSuppressResume  = 0.70
	suppressResumeScale    = 0.8
)

var (
	errMetricsClientRequired = errors.New("adapt: metrics client is required")
	errDutyCyclerRequired    = errors.New("adapt: duty cycler is required")
	// ErrInvalidConfig signals that the supplied controller configuration is invalid.
	ErrInvalidConfig = errors.New("adapt: invalid config")
)

// DefaultConfig mirrors the initial implementation plan for control loop cadence.
func DefaultConfig() Config {
	return Config{
		ResourceID:        "",
		Mode:              defaultModeLabel,
		TargetStart:       defaultTargetStart,
		TargetMin:         defaultTargetMin,
		TargetMax:         defaultTargetMax,
		StepUp:            defaultStepUp,
		StepDown:          defaultStepDown,
		FallbackTarget:    defaultFallbackTarget,
		GoalLow:           defaultGoalLow,
		GoalHigh:          defaultGoalHigh,
		Interval:          time.Hour,
		RelaxedInterval:   defaultRelaxedInterval,
		RelaxedThreshold:  defaultRelaxedThresh,
		SuppressThreshold: defaultSuppressThresh,
		SuppressResume:    defaultSuppressResume,
	}
}

// ModeEnforcesTargets reports whether the provided controller mode should mutate
// the worker pool duty cycle.
func ModeEnforcesTargets(mode string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	if trimmed == "" {
		trimmed = defaultModeLabel
	}

	switch trimmed {
	case dryRunModeLabel, noopModeLabel:
		return false
	default:
		return true
	}
}

func normalizeConfig(cfg Config) (Config, string, error) {
	normalized, mode := coerceConfig(cfg)

	err := validateControllerConfig(normalized)
	if err != nil {
		return Config{}, "", err
	}

	return normalized, mode, nil
}

// ValidateConfig ensures controller thresholds are internally consistent.
func ValidateConfig(cfg Config) error {
	normalized, _ := coerceConfig(cfg)

	return validateControllerConfig(normalized)
}

func coerceConfig(cfg Config) (Config, string) {
	defaults := DefaultConfig()

	cfg.Interval = ensureDuration(cfg.Interval, defaults.Interval)
	cfg.RelaxedInterval = ensureDuration(cfg.RelaxedInterval, defaults.RelaxedInterval)
	cfg.TargetStart = ensureFloat(cfg.TargetStart, defaults.TargetStart)
	cfg.TargetMin = ensureFloat(cfg.TargetMin, defaults.TargetMin)
	cfg.TargetMax = ensureFloat(cfg.TargetMax, defaults.TargetMax)
	cfg.StepUp = ensureFloat(cfg.StepUp, defaults.StepUp)
	cfg.StepDown = ensureFloat(cfg.StepDown, defaults.StepDown)
	cfg.FallbackTarget = ensureFloat(cfg.FallbackTarget, defaults.FallbackTarget)
	cfg.GoalLow = ensureFloat(cfg.GoalLow, defaults.GoalLow)
	cfg.GoalHigh = ensureFloat(cfg.GoalHigh, defaults.GoalHigh)
	cfg.RelaxedThreshold = ensureFloat(cfg.RelaxedThreshold, defaults.RelaxedThreshold)
	cfg.SuppressThreshold = ensureFloatAllowZero(cfg.SuppressThreshold, defaults.SuppressThreshold)
	cfg.SuppressResume = ensureFloatAllowZero(cfg.SuppressResume, defaults.SuppressResume)

	cfg.SuppressThreshold = clamp(cfg.SuppressThreshold, 0, 1)
	cfg.SuppressResume = clamp(cfg.SuppressResume, 0, 1)

	if cfg.SuppressThreshold <= 0 {
		cfg.SuppressResume = 0
	} else if cfg.SuppressResume >= cfg.SuppressThreshold {
		cfg.SuppressResume = math.Max(cfg.SuppressThreshold*suppressResumeScale, 0)
	}

	mode := strings.TrimSpace(cfg.Mode)
	if mode == "" {
		mode = defaultModeLabel
	}

	return cfg, mode
}

func validateControllerConfig(cfg Config) error {
	thresholds := []struct {
		name  string
		value float64
	}{
		{"controller.targetStart", cfg.TargetStart},
		{"controller.targetMin", cfg.TargetMin},
		{"controller.targetMax", cfg.TargetMax},
		{"controller.fallbackTarget", cfg.FallbackTarget},
		{"controller.goalLow", cfg.GoalLow},
		{"controller.goalHigh", cfg.GoalHigh},
	}

	for _, threshold := range thresholds {
		if cfg.SuppressThreshold > 0 && cfg.SuppressThreshold <= threshold.value {
			return fmt.Errorf(
				"%w: controller.suppressThreshold (%.2f) must be greater than %s (%.2f)",
				ErrInvalidConfig,
				cfg.SuppressThreshold,
				threshold.name,
				threshold.value,
			)
		}

		if cfg.SuppressResume > 0 && cfg.SuppressResume <= threshold.value {
			return fmt.Errorf(
				"%w: controller.suppressResume (%.2f) must be greater than %s (%.2f)",
				ErrInvalidConfig,
				cfg.SuppressResume,
				threshold.name,
				threshold.value,
			)
		}
	}

	return nil
}

func ensureDuration(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}

	return value
}

func ensureFloat(value, fallback float64) float64 {
	if value == 0 {
		return fallback
	}

	return value
}

func ensureFloatAllowZero(value, fallback float64) float64 {
	if value == 0 {
		return 0
	}

	return ensureFloat(value, fallback)
}

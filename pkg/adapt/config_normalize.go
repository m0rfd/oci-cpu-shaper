package adapt

import (
	"math"
	"strings"
	"time"
)

func normalizeConfig(cfg Config) (Config, string, error) {
	normalized, mode := coerceConfig(cfg)

	err := validateControllerConfig(normalized)
	if err != nil {
		return Config{}, "", err
	}

	return normalized, mode, nil
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
	cfg.RelaxedConfirmations = ensureInt(cfg.RelaxedConfirmations, defaults.RelaxedConfirmations)
	cfg.SuppressThreshold = ensureFloatAllowZero(cfg.SuppressThreshold, defaults.SuppressThreshold)
	cfg.SuppressResume = ensureFloatAllowZero(cfg.SuppressResume, defaults.SuppressResume)
	cfg.SuppressRunnableThreshold = ensureFloatAllowZero(
		cfg.SuppressRunnableThreshold,
		defaults.SuppressRunnableThreshold,
	)
	cfg.SuppressRunnableResume = ensureFloatAllowZero(
		cfg.SuppressRunnableResume,
		defaults.SuppressRunnableResume,
	)
	cfg.SuppressSmoothingSamples = ensureIntAllowZero(
		cfg.SuppressSmoothingSamples,
		defaults.SuppressSmoothingSamples,
	)

	cfg.SuppressThreshold = clamp(cfg.SuppressThreshold, 0, 1)

	cfg.SuppressResume = clamp(cfg.SuppressResume, 0, 1)
	if cfg.SuppressRunnableThreshold < 0 {
		cfg.SuppressRunnableThreshold = 0
	}

	if cfg.SuppressRunnableResume < 0 {
		cfg.SuppressRunnableResume = 0
	}

	if cfg.SuppressThreshold <= 0 {
		cfg.SuppressResume = 0
	} else if cfg.SuppressResume >= cfg.SuppressThreshold {
		cfg.SuppressResume = math.Max(cfg.SuppressThreshold*suppressResumeScale, 0)
	}

	if cfg.SuppressRunnableThreshold <= 0 {
		cfg.SuppressRunnableResume = 0
	} else if cfg.SuppressRunnableResume >= cfg.SuppressRunnableThreshold {
		cfg.SuppressRunnableResume = cfg.SuppressRunnableThreshold * suppressResumeScale
	}

	mode := normalizeModeLabel(cfg.Mode)
	cfg.Mode = mode

	return cfg, mode
}

func normalizeModeLabel(mode string) string {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	if trimmed == "" || trimmed == legacyEnforceModeLabel {
		return enforceModeLabel
	}

	return trimmed
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

func ensureInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}

	return value
}

func ensureIntAllowZero(value, fallback int) int {
	if value == 0 {
		return 0
	}

	return ensureInt(value, fallback)
}

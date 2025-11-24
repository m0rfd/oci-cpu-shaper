package adapt

import (
	"strings"
	"time"
)

const (
	defaultModeLabel       = "normal"
	dryRunModeLabel        = "dry-run"
	noopModeLabel          = "noop"
	defaultTargetStart     = 0.22
	defaultTargetMin       = 0.20
	defaultTargetMax       = 0.32
	defaultStepUp          = 0.01
	defaultStepDown        = 0.005
	defaultFallbackTarget  = 0.22
	defaultGoalLow         = 0.21
	defaultGoalHigh        = 0.27
	defaultRelaxedInterval = 4 * time.Hour
	defaultRelaxedThresh   = 0.26
	defaultSuppressThresh  = 0.80
	defaultSuppressResume  = 0.68
	suppressResumeScale    = 0.8
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

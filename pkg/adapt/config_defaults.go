package adapt

import "time"

const (
	enforceModeLabel              = "enforce"
	legacyEnforceModeLabel        = "normal"
	defaultModeLabel              = enforceModeLabel
	dryRunModeLabel               = "dry-run"
	noopModeLabel                 = "noop"
	defaultTargetStart            = 0.22
	defaultTargetMin              = 0.20
	defaultTargetMax              = 0.32
	defaultStepUp                 = 0.01
	defaultStepDown               = 0.005
	defaultFallbackTarget         = 0.22
	defaultGoalLow                = 0.21
	defaultGoalHigh               = 0.27
	defaultRelaxedInterval        = 4 * time.Hour
	defaultRelaxedThresh          = 0.26
	defaultRelaxedConfirmations   = 2
	defaultSuppressThresh         = 0.80
	defaultSuppressResume         = 0.68
	defaultRunnableSuppressThresh = 1.20
	defaultRunnableSuppressResume = 0.96
	suppressResumeScale           = 0.8
)

// DefaultConfig reflects the current controller defaults; keep this aligned with
// docs/initial-implementation-plan.md.
func DefaultConfig() Config {
	return Config{
		ResourceID:                "",
		Mode:                      defaultModeLabel,
		TargetStart:               defaultTargetStart,
		TargetMin:                 defaultTargetMin,
		TargetMax:                 defaultTargetMax,
		StepUp:                    defaultStepUp,
		StepDown:                  defaultStepDown,
		FallbackTarget:            defaultFallbackTarget,
		GoalLow:                   defaultGoalLow,
		GoalHigh:                  defaultGoalHigh,
		Interval:                  time.Hour,
		RelaxedInterval:           defaultRelaxedInterval,
		RelaxedThreshold:          defaultRelaxedThresh,
		RelaxedConfirmations:      defaultRelaxedConfirmations,
		SuppressThreshold:         defaultSuppressThresh,
		SuppressResume:            defaultSuppressResume,
		SuppressRunnableThreshold: defaultRunnableSuppressThresh,
		SuppressRunnableResume:    defaultRunnableSuppressResume,
	}
}

// ModeEnforcesTargets reports whether the provided controller mode should mutate
// the worker pool duty cycle.
func ModeEnforcesTargets(mode string) bool {
	trimmed := normalizeModeLabel(mode)

	switch trimmed {
	case dryRunModeLabel, noopModeLabel:
		return false
	default:
		return true
	}
}

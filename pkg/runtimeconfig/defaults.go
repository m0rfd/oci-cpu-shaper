package runtimeconfig

import (
	"time"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/shape"
)

const (
	defaultEstimatorInterval = 1 * time.Second
	defaultPoolWorkers       = 2
)

// Default returns the fully merged default runtime configuration.
func Default() Config {
	defaults := adapt.DefaultConfig()

	var cfg Config

	cfg.Controller.TargetStart = defaults.TargetStart
	cfg.Controller.TargetMin = defaults.TargetMin
	cfg.Controller.TargetMax = defaults.TargetMax
	cfg.Controller.StepUp = defaults.StepUp
	cfg.Controller.StepDown = defaults.StepDown
	cfg.Controller.FallbackTarget = defaults.FallbackTarget
	cfg.Controller.GoalLow = defaults.GoalLow
	cfg.Controller.GoalHigh = defaults.GoalHigh
	cfg.Controller.Interval = defaults.Interval
	cfg.Controller.RelaxedInterval = defaults.RelaxedInterval
	cfg.Controller.RelaxedThreshold = defaults.RelaxedThreshold
	cfg.Controller.RelaxedConfirmations = defaults.RelaxedConfirmations
	cfg.Controller.SuppressThreshold = defaults.SuppressThreshold
	cfg.Controller.SuppressResume = defaults.SuppressResume
	cfg.Controller.SuppressRunnableThreshold = defaults.SuppressRunnableThreshold
	cfg.Controller.SuppressRunnableResume = defaults.SuppressRunnableResume
	cfg.Controller.SuppressSmoothingSamples = defaults.SuppressSmoothingSamples

	cfg.Estimator.Interval = defaultEstimatorInterval

	cfg.Pool.Workers = defaultPoolWorkers
	cfg.Pool.AutoSizeFromShape = false

	cfg.Pool.Quantum = shape.DefaultQuantum
	cfg.Pool.PauseThreshold = defaults.SuppressThreshold
	cfg.Pool.ResumeThreshold = defaults.SuppressResume
	cfg.Pool.RunnableGuard = defaults.SuppressRunnableThreshold

	cfg.HTTP.Bind = ":9108"

	return cfg
}

// ToAdaptConfig maps the controller configuration into the adapt.Config struct.
func (cfg Config) ToAdaptConfig() adapt.Config {
	return adapt.Config{
		ResourceID:                "",
		Mode:                      "",
		TargetStart:               cfg.Controller.TargetStart,
		TargetMin:                 cfg.Controller.TargetMin,
		TargetMax:                 cfg.Controller.TargetMax,
		StepUp:                    cfg.Controller.StepUp,
		StepDown:                  cfg.Controller.StepDown,
		FallbackTarget:            cfg.Controller.FallbackTarget,
		GoalLow:                   cfg.Controller.GoalLow,
		GoalHigh:                  cfg.Controller.GoalHigh,
		Interval:                  cfg.Controller.Interval,
		RelaxedInterval:           cfg.Controller.RelaxedInterval,
		RelaxedThreshold:          cfg.Controller.RelaxedThreshold,
		RelaxedConfirmations:      cfg.Controller.RelaxedConfirmations,
		SuppressThreshold:         cfg.Controller.SuppressThreshold,
		SuppressResume:            cfg.Controller.SuppressResume,
		SuppressRunnableThreshold: cfg.Controller.SuppressRunnableThreshold,
		SuppressRunnableResume:    cfg.Controller.SuppressRunnableResume,
		SuppressSmoothingSamples:  cfg.Controller.SuppressSmoothingSamples,
	}
}

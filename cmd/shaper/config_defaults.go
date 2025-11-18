package main

import (
	"runtime"
	"time"

	"oci-cpu-shaper/pkg/adapt"
	"oci-cpu-shaper/pkg/shape"
)

func defaultRuntimeConfig() runtimeConfig {
	defaults := adapt.DefaultConfig()

	var cfg runtimeConfig

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
	cfg.Controller.SuppressThreshold = defaults.SuppressThreshold
	cfg.Controller.SuppressResume = defaults.SuppressResume

	cfg.Estimator.Interval = time.Second

	cfg.Pool.Workers = runtime.NumCPU()
	if cfg.Pool.Workers <= 0 {
		cfg.Pool.Workers = 1
	}

	cfg.Pool.Quantum = shape.DefaultQuantum
	cfg.Pool.PauseThreshold = defaults.SuppressThreshold
	cfg.Pool.ResumeThreshold = defaults.SuppressResume

	cfg.HTTP.Bind = ":9108"

	return cfg
}

func runtimeToAdaptControllerConfig(cfg runtimeConfig) adapt.Config {
	return adapt.Config{
		ResourceID:        "",
		Mode:              "",
		TargetStart:       cfg.Controller.TargetStart,
		TargetMin:         cfg.Controller.TargetMin,
		TargetMax:         cfg.Controller.TargetMax,
		StepUp:            cfg.Controller.StepUp,
		StepDown:          cfg.Controller.StepDown,
		FallbackTarget:    cfg.Controller.FallbackTarget,
		GoalLow:           cfg.Controller.GoalLow,
		GoalHigh:          cfg.Controller.GoalHigh,
		Interval:          cfg.Controller.Interval,
		RelaxedInterval:   cfg.Controller.RelaxedInterval,
		RelaxedThreshold:  cfg.Controller.RelaxedThreshold,
		SuppressThreshold: cfg.Controller.SuppressThreshold,
		SuppressResume:    cfg.Controller.SuppressResume,
	}
}

package main

import (
	"testing"
	"time"
)

func TestDefaultRuntimeConfigAlignsWithAdaptDefaults(t *testing.T) {
	t.Parallel()

	cfg := defaultRuntimeConfig()
	defaults := adaptDefault()

	assertFloatEqual(t, "targetStart", cfg.Controller.TargetStart, defaults.TargetStart)
	assertFloatEqual(t, "targetMin", cfg.Controller.TargetMin, defaults.TargetMin)
	assertFloatEqual(t, "targetMax", cfg.Controller.TargetMax, defaults.TargetMax)
	assertFloatEqual(t, "stepUp", cfg.Controller.StepUp, defaults.StepUp)
	assertFloatEqual(t, "stepDown", cfg.Controller.StepDown, defaults.StepDown)
	assertFloatEqual(t, "fallbackTarget", cfg.Controller.FallbackTarget, defaults.FallbackTarget)
	assertFloatEqual(t, "goalLow", cfg.Controller.GoalLow, defaults.GoalLow)
	assertFloatEqual(t, "goalHigh", cfg.Controller.GoalHigh, defaults.GoalHigh)
	assertDurationEqual(t, "interval", cfg.Controller.Interval, defaults.Interval)
	assertDurationEqual(
		t,
		"relaxedInterval",
		cfg.Controller.RelaxedInterval,
		defaults.RelaxedInterval,
	)
	assertFloatEqual(
		t,
		"relaxedThreshold",
		cfg.Controller.RelaxedThreshold,
		defaults.RelaxedThreshold,
	)
	assertFloatEqual(
		t,
		"suppressThreshold",
		cfg.Controller.SuppressThreshold,
		defaults.SuppressThreshold,
	)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, defaults.SuppressResume)

	assertDurationEqual(t, "estimatorInterval", cfg.Estimator.Interval, time.Second)

	if cfg.Pool.Workers <= 0 {
		t.Fatalf("expected positive worker count, got %d", cfg.Pool.Workers)
	}

	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, defaults.SuppressThreshold)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, defaults.SuppressResume)
	assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9108")
}

func TestRuntimeToAdaptControllerConfig(t *testing.T) {
	t.Parallel()

	cfg := runtimeConfig{ //nolint:exhaustruct
		Controller: controllerConfig{
			TargetStart:       0.45,
			TargetMin:         0.3,
			TargetMax:         0.6,
			StepUp:            0.02,
			StepDown:          0.01,
			FallbackTarget:    0.4,
			GoalLow:           0.35,
			GoalHigh:          0.5,
			Interval:          time.Minute,
			RelaxedInterval:   30 * time.Minute,
			RelaxedThreshold:  0.2,
			SuppressThreshold: 0.9,
			SuppressResume:    0.6,
		},
	}

	adaptCfg := runtimeToAdaptControllerConfig(cfg)

	assertFloatEqual(t, "targetStart", adaptCfg.TargetStart, cfg.Controller.TargetStart)
	assertFloatEqual(t, "targetMin", adaptCfg.TargetMin, cfg.Controller.TargetMin)
	assertFloatEqual(t, "targetMax", adaptCfg.TargetMax, cfg.Controller.TargetMax)
	assertFloatEqual(t, "stepUp", adaptCfg.StepUp, cfg.Controller.StepUp)
	assertFloatEqual(t, "stepDown", adaptCfg.StepDown, cfg.Controller.StepDown)
	assertFloatEqual(t, "fallbackTarget", adaptCfg.FallbackTarget, cfg.Controller.FallbackTarget)
	assertFloatEqual(t, "goalLow", adaptCfg.GoalLow, cfg.Controller.GoalLow)
	assertFloatEqual(t, "goalHigh", adaptCfg.GoalHigh, cfg.Controller.GoalHigh)
	assertDurationEqual(t, "interval", adaptCfg.Interval, cfg.Controller.Interval)
	assertDurationEqual(
		t,
		"relaxedInterval",
		adaptCfg.RelaxedInterval,
		cfg.Controller.RelaxedInterval,
	)
	assertFloatEqual(
		t,
		"relaxedThreshold",
		adaptCfg.RelaxedThreshold,
		cfg.Controller.RelaxedThreshold,
	)
	assertFloatEqual(
		t,
		"suppressThreshold",
		adaptCfg.SuppressThreshold,
		cfg.Controller.SuppressThreshold,
	)
	assertFloatEqual(t, "suppressResume", adaptCfg.SuppressResume, cfg.Controller.SuppressResume)
}

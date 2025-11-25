//nolint:testpackage // tests require access to unexported config helpers.
package runtimeconfig

import (
	"testing"
	"time"

	"oci-cpu-shaper/pkg/adapt"
)

func TestDefaultConfigAlignsWithAdaptDefaults(t *testing.T) {
	t.Parallel()

	cfg := Default()
	defaults := adaptDefault()

	assertDefaultControllerFields(t, cfg, defaults)
	assertDurationEqual(t, "estimatorInterval", cfg.Estimator.Interval, time.Second)
	assertIntEqual(t, "poolWorkers", cfg.Pool.Workers, 2)
	assertBoolEqual(t, "poolAutoSizeFromShape", cfg.Pool.AutoSizeFromShape, false)
	assertDefaultPoolFields(t, cfg, defaults)
	assertStringEqual(t, "httpBind", cfg.HTTP.Bind, ":9108")
}

func assertDefaultControllerFields(t *testing.T, cfg Config, defaults adapt.Config) {
	t.Helper()

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
	assertIntEqual(
		t,
		"relaxedConfirmations",
		cfg.Controller.RelaxedConfirmations,
		defaults.RelaxedConfirmations,
	)
	assertFloatEqual(
		t,
		"suppressThreshold",
		cfg.Controller.SuppressThreshold,
		defaults.SuppressThreshold,
	)
	assertFloatEqual(t, "suppressResume", cfg.Controller.SuppressResume, defaults.SuppressResume)
	assertFloatEqual(
		t,
		"suppressRunnableThreshold",
		cfg.Controller.SuppressRunnableThreshold,
		defaults.SuppressRunnableThreshold,
	)
	assertFloatEqual(
		t,
		"suppressRunnableResume",
		cfg.Controller.SuppressRunnableResume,
		defaults.SuppressRunnableResume,
	)
	assertIntEqual(
		t,
		"suppressSmoothingSamples",
		cfg.Controller.SuppressSmoothingSamples,
		defaults.SuppressSmoothingSamples,
	)
}

func assertDefaultPoolFields(t *testing.T, cfg Config, defaults adapt.Config) {
	t.Helper()

	assertFloatEqual(t, "poolPauseThreshold", cfg.Pool.PauseThreshold, defaults.SuppressThreshold)
	assertFloatEqual(t, "poolResumeThreshold", cfg.Pool.ResumeThreshold, defaults.SuppressResume)
	assertFloatEqual(
		t,
		"poolRunnableGuard",
		cfg.Pool.RunnableGuard,
		defaults.SuppressRunnableThreshold,
	)
}

func TestConfigToAdaptConfig(t *testing.T) {
	t.Parallel()

	cfg := Config{ //nolint:exhaustruct
		Controller: ControllerConfig{
			TargetStart:               0.45,
			TargetMin:                 0.3,
			TargetMax:                 0.6,
			StepUp:                    0.02,
			StepDown:                  0.01,
			FallbackTarget:            0.4,
			GoalLow:                   0.35,
			GoalHigh:                  0.5,
			Interval:                  time.Minute,
			RelaxedInterval:           30 * time.Minute,
			RelaxedThreshold:          0.2,
			RelaxedConfirmations:      4,
			SuppressThreshold:         0.9,
			SuppressResume:            0.6,
			SuppressRunnableThreshold: 1.4,
			SuppressRunnableResume:    1.1,
			SuppressSmoothingSamples:  3,
		},
	}

	adaptCfg := cfg.ToAdaptConfig()

	assertAdaptConfigMapping(t, cfg.Controller, adaptCfg)
}

func assertAdaptConfigMapping(t *testing.T, controllerCfg ControllerConfig, adaptCfg adapt.Config) {
	t.Helper()

	assertAdaptFloatFields(t, controllerCfg, adaptCfg)
	assertAdaptDurationFields(t, controllerCfg, adaptCfg)
	assertAdaptIntFields(t, controllerCfg, adaptCfg)
}

func assertAdaptFloatFields(t *testing.T, controllerCfg ControllerConfig, adaptCfg adapt.Config) {
	t.Helper()

	floatChecks := []struct {
		name string
		got  float64
		want float64
	}{
		{name: "targetStart", got: adaptCfg.TargetStart, want: controllerCfg.TargetStart},
		{name: "targetMin", got: adaptCfg.TargetMin, want: controllerCfg.TargetMin},
		{name: "targetMax", got: adaptCfg.TargetMax, want: controllerCfg.TargetMax},
		{name: "stepUp", got: adaptCfg.StepUp, want: controllerCfg.StepUp},
		{name: "stepDown", got: adaptCfg.StepDown, want: controllerCfg.StepDown},
		{name: "fallbackTarget", got: adaptCfg.FallbackTarget, want: controllerCfg.FallbackTarget},
		{name: "goalLow", got: adaptCfg.GoalLow, want: controllerCfg.GoalLow},
		{name: "goalHigh", got: adaptCfg.GoalHigh, want: controllerCfg.GoalHigh},
		{
			name: "relaxedThreshold",
			got:  adaptCfg.RelaxedThreshold,
			want: controllerCfg.RelaxedThreshold,
		},
		{
			name: "suppressThreshold",
			got:  adaptCfg.SuppressThreshold,
			want: controllerCfg.SuppressThreshold,
		},
		{name: "suppressResume", got: adaptCfg.SuppressResume, want: controllerCfg.SuppressResume},
		{
			name: "suppressRunnableThreshold",
			got:  adaptCfg.SuppressRunnableThreshold,
			want: controllerCfg.SuppressRunnableThreshold,
		},
		{
			name: "suppressRunnableResume",
			got:  adaptCfg.SuppressRunnableResume,
			want: controllerCfg.SuppressRunnableResume,
		},
	}

	for _, tt := range floatChecks {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertFloatEqual(t, tt.name, tt.got, tt.want)
		})
	}
}

func assertAdaptDurationFields(
	t *testing.T,
	controllerCfg ControllerConfig,
	adaptCfg adapt.Config,
) {
	t.Helper()

	durationChecks := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "interval", got: adaptCfg.Interval, want: controllerCfg.Interval},
		{
			name: "relaxedInterval",
			got:  adaptCfg.RelaxedInterval,
			want: controllerCfg.RelaxedInterval,
		},
	}

	for _, tt := range durationChecks {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertDurationEqual(t, tt.name, tt.got, tt.want)
		})
	}
}

func assertAdaptIntFields(t *testing.T, controllerCfg ControllerConfig, adaptCfg adapt.Config) {
	t.Helper()

	intChecks := []struct {
		name string
		got  int
		want int
	}{
		{
			name: "relaxedConfirmations",
			got:  adaptCfg.RelaxedConfirmations,
			want: controllerCfg.RelaxedConfirmations,
		},
		{
			name: "suppressSmoothingSamples",
			got:  adaptCfg.SuppressSmoothingSamples,
			want: controllerCfg.SuppressSmoothingSamples,
		},
	}

	for _, tt := range intChecks {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertIntEqual(t, tt.name, tt.got, tt.want)
		})
	}
}

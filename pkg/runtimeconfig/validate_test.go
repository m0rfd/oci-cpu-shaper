//nolint:testpackage // tests require access to unexported config helpers.
package runtimeconfig

import "testing"

func TestValidateRuntimeConfigRejectsInvalidBounds(t *testing.T) {
	t.Parallel()

	makeConfig := func(mod func(*Config)) Config {
		cfg := Default()
		mod(&cfg)

		return cfg
	}

	testCases := []struct {
		name    string
		cfg     Config
		wantRef string
	}{
		{
			name: "target bounds",
			cfg: makeConfig(func(cfg *Config) {
				cfg.Controller.TargetMin = 0.45
				cfg.Controller.TargetMax = 0.30
			}),
			wantRef: "controller.targetMin",
		},
		{
			name: "target start above max",
			cfg: makeConfig(func(cfg *Config) {
				cfg.Controller.TargetStart = 0.60
				cfg.Controller.TargetMax = 0.50
			}),
			wantRef: "controller.targetStart",
		},
		{
			name: "goalLow above goalHigh",
			cfg: makeConfig(func(cfg *Config) {
				cfg.Controller.GoalLow = 0.35
				cfg.Controller.GoalHigh = 0.30
			}),
			wantRef: "controller.goalLow",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateRuntimeConfig(tc.cfg)
			assertInvalidRuntimeConfigError(t, err, tc.wantRef)
		})
	}
}

func TestValidateRuntimeConfigRejectsNonPositiveValues(t *testing.T) {
	t.Parallel()

	makeConfig := func(mod func(*Config)) Config {
		cfg := Default()
		mod(&cfg)

		return cfg
	}

	testCases := []struct {
		name    string
		cfg     Config
		wantRef string
	}{
		{
			name:    "zero controller interval",
			cfg:     makeConfig(func(cfg *Config) { cfg.Controller.Interval = 0 }),
			wantRef: "controller.interval",
		},
		{
			name:    "zero worker count",
			cfg:     makeConfig(func(cfg *Config) { cfg.Pool.Workers = 0 }),
			wantRef: "pool.workers",
		},
		{
			name: "negative steps",
			cfg: makeConfig(func(cfg *Config) {
				cfg.Controller.StepUp = -0.01
				cfg.Controller.StepDown = -0.02
			}),
			wantRef: "controller.stepUp",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateRuntimeConfig(tc.cfg)
			assertInvalidRuntimeConfigError(t, err, tc.wantRef)
		})
	}
}

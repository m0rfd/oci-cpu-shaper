// Package adapt validates controller signal recording helpers to keep the
// interval selection logic exercised alongside recorder expectations.
//
//nolint:testpackage,godoclint // Tests require controller internals and doc coverage.
package adapt

import (
	"context"
	"testing"
	"time"
)

func TestAdaptiveControllerRecordsIntervalAcrossP95Branches(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Interval = time.Minute
	cfg.RelaxedInterval = time.Hour

	tests := []struct {
		name         string
		p95          float64
		wantInterval time.Duration
		wantState    State
	}{
		{
			name:         "hot-workload-uses-relaxed-interval",
			p95:          cfg.RelaxedThreshold,
			wantInterval: cfg.RelaxedInterval,
			wantState:    StateNormal,
		},
		{
			name:         "healthy-workload-keeps-fast-interval",
			p95:          cfg.RelaxedThreshold - 0.05,
			wantInterval: cfg.Interval,
			wantState:    StateNormal,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := newStubMetricsRecorder()
			metrics := newFakeMetrics([]metricResult{{value: testCase.p95, err: nil}})
			shaper := newFakeShaper()

			controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
			if err != nil {
				t.Fatalf("NewAdaptiveController: %v", err)
			}

			stepper, ok := any(controller).(controllerStepper)
			if !ok {
				t.Fatalf("controller does not expose stepper interface")
			}

			interval := stepper.step(context.Background())

			requireEqual(t, "step interval", interval, testCase.wantInterval)
			requireEqual(t, "recorded interval", recorder.interval, testCase.wantInterval)
			requireEqual(t, "state", controller.State(), testCase.wantState)
		})
	}
}

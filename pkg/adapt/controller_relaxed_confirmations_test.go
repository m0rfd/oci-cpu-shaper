//nolint:testpackage // Tests need internal helpers like runIntervalRecordingScenario.
package adapt

import (
	"context"
	"testing"
	"time"
)

// TestRelaxedConfirmationsCounterIncrement verifies that the relaxedSuccesses
// counter increments correctly when P95 is at or above the threshold.
func TestRelaxedConfirmationsCounterIncrement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		confirmations     int
		p95Samples        []float64
		threshold         float64
		expectedIntervals []time.Duration
	}{
		{
			name:          "immediate-switch-with-one-confirmation",
			confirmations: 1,
			p95Samples:    []float64{0.30},
			threshold:     0.26,
			expectedIntervals: []time.Duration{
				4 * time.Hour, // Should switch immediately
			},
		},
		{
			name:          "two-confirmations-default",
			confirmations: 2,
			p95Samples:    []float64{0.30, 0.27},
			threshold:     0.26,
			expectedIntervals: []time.Duration{
				time.Hour,     // First high: still normal
				4 * time.Hour, // Second consecutive: switch to relaxed
			},
		},
		{
			name:          "five-confirmations-slow-switch",
			confirmations: 5,
			p95Samples:    []float64{0.30, 0.28, 0.27, 0.29, 0.31},
			threshold:     0.26,
			expectedIntervals: []time.Duration{
				time.Hour,     // 1st
				time.Hour,     // 2nd
				time.Hour,     // 3rd
				time.Hour,     // 4th
				4 * time.Hour, // 5th consecutive: switch
			},
		},
		{
			name:          "exactly-at-threshold",
			confirmations: 2,
			p95Samples:    []float64{0.26, 0.26},
			threshold:     0.26,
			expectedIntervals: []time.Duration{
				time.Hour,
				4 * time.Hour, // At threshold counts as high
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			runConfirmationIncrementTest(t, testCase.confirmations, testCase.threshold,
				testCase.p95Samples, testCase.expectedIntervals)
		})
	}
}

func runConfirmationIncrementTest(
	t *testing.T,
	confirmations int,
	threshold float64,
	p95Samples []float64,
	expectedIntervals []time.Duration,
) {
	t.Helper()

	cfg := DefaultConfig()
	cfg.Interval = time.Hour
	cfg.RelaxedInterval = 4 * time.Hour
	cfg.RelaxedThreshold = threshold
	cfg.RelaxedConfirmations = confirmations

	results := make([]metricResult, len(p95Samples))
	for index, p95 := range p95Samples {
		results[index] = metricResult{value: p95, err: nil}
	}

	runIntervalRecordingScenario(
		t,
		cfg,
		results,
		expectedIntervals,
		StateNormal,
	)
}

// TestRelaxedConfirmationsCounterReset verifies that the counter resets
// to zero when P95 drops below the threshold.
func TestRelaxedConfirmationsCounterReset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		p95Samples        []float64
		expectedIntervals []time.Duration
	}{
		{
			name: "reset-on-drop-before-confirmation",
			p95Samples: []float64{
				0.30, // 1st high
				0.24, // Drop below threshold - reset counter
				0.30, // 1st high again (counter was reset)
				0.28, // 2nd consecutive high
			},
			expectedIntervals: []time.Duration{
				time.Hour,     // 1st high, counter = 1
				time.Hour,     // Dropped, counter reset to 0
				time.Hour,     // 1st high again, counter = 1
				4 * time.Hour, // 2nd consecutive, counter = 2, switch
			},
		},
		{
			name: "reset-after-entering-relaxed-mode",
			p95Samples: []float64{
				0.30, // 1st high
				0.28, // 2nd consecutive high - enter relaxed
				0.24, // Drop - reset counter
				0.30, // Back to high, but counter reset
				0.29, // 2nd consecutive again
			},
			expectedIntervals: []time.Duration{
				time.Hour,     // 1st high
				4 * time.Hour, // Entered relaxed mode
				time.Hour,     // Dropped, back to normal interval
				time.Hour,     // 1st high after reset
				4 * time.Hour, // 2nd consecutive
			},
		},
		{
			name: "just-below-threshold-resets",
			p95Samples: []float64{
				0.30,  // High
				0.259, // Just below 0.26 threshold
				0.30,  // Back to high
				0.28,  // Still high
			},
			expectedIntervals: []time.Duration{
				time.Hour,     // 1st
				time.Hour,     // Dropped slightly, reset
				time.Hour,     // 1st again
				4 * time.Hour, // 2nd consecutive
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			cfg.Interval = time.Hour
			cfg.RelaxedInterval = 4 * time.Hour
			cfg.RelaxedThreshold = 0.26
			cfg.RelaxedConfirmations = 2

			results := make([]metricResult, len(testCase.p95Samples))
			for index, p95 := range testCase.p95Samples {
				results[index] = metricResult{value: p95, err: nil}
			}

			runIntervalRecordingScenario(
				t,
				cfg,
				results,
				testCase.expectedIntervals,
				StateNormal,
			)
		})
	}
}

// TestRelaxedConfirmationsResetOnError verifies that errors reset the counter.
func TestRelaxedConfirmationsResetOnError(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Interval = time.Hour
	cfg.RelaxedInterval = 4 * time.Hour
	cfg.RelaxedThreshold = 0.26
	cfg.RelaxedConfirmations = 2

	results := []metricResult{
		{value: 0.30, err: nil},     // 1st high
		{value: 0, err: errOCIDown}, // Error - should reset counter
		{value: 0.30, err: nil},     // 1st high again after error
		{value: 0.28, err: nil},     // 2nd consecutive high
	}

	expectedIntervals := []time.Duration{
		time.Hour,     // 1st high
		time.Hour,     // Error, back to fallback with normal interval
		time.Hour,     // Recovery, 1st high (counter reset by error)
		4 * time.Hour, // 2nd consecutive, enter relaxed
	}

	recorder := newStubMetricsRecorder()
	metrics := newFakeMetrics(results)
	shaper := newFakeShaper()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatalf("controller does not expose stepper interface")
	}

	for index, wantInterval := range expectedIntervals {
		interval := stepper.step(context.Background())

		if interval != wantInterval {
			t.Fatalf(
				"step %d interval: got %v want %v",
				index,
				interval,
				wantInterval,
			)
		}

		// Verify state is correct (fallback on error, normal otherwise)
		expectedState := StateNormal
		if results[index].err != nil {
			expectedState = StateFallback
		}

		if controller.State() != expectedState {
			t.Fatalf(
				"step %d state: got %v want %v",
				index,
				controller.State(),
				expectedState,
			)
		}
	}
}

// TestRelaxedConfirmationsStaysInRelaxedMode verifies that once in relaxed mode,
// the controller stays there as long as P95 stays above threshold.
func TestRelaxedConfirmationsStaysInRelaxedMode(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Interval = time.Hour
	cfg.RelaxedInterval = 4 * time.Hour
	cfg.RelaxedThreshold = 0.26
	cfg.RelaxedConfirmations = 2

	// Long sequence of high P95 values
	p95Samples := []float64{0.30, 0.28, 0.27, 0.29, 0.31, 0.26, 0.28}
	expectedIntervals := []time.Duration{
		time.Hour,     // 1st high
		4 * time.Hour, // 2nd consecutive, enter relaxed
		4 * time.Hour, // Stay in relaxed
		4 * time.Hour, // Stay in relaxed
		4 * time.Hour, // Stay in relaxed
		4 * time.Hour, // At threshold, stay in relaxed
		4 * time.Hour, // Stay in relaxed
	}

	results := make([]metricResult, len(p95Samples))
	for index, p95 := range p95Samples {
		results[index] = metricResult{value: p95, err: nil}
	}

	runIntervalRecordingScenario(
		t,
		cfg,
		results,
		expectedIntervals,
		StateNormal,
	)
}

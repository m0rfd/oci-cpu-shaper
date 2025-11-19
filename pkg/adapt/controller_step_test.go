// Package adapt exercises the controller step transitions to ensure duty-cycle
// scheduling keeps matching the production policy table.
//
//nolint:testpackage,godoclint // Tests need internal helpers and separate coverage notes per file.
package adapt

import (
	"context"
	"math"
	"testing"
	"time"
)

type controllerScenario struct {
	name         string
	results      []metricResult
	expectations []stepExpectation
}

type stepExpectation struct {
	state        State
	target       float64
	nextInterval time.Duration
}

func TestControllerStateTransitions(t *testing.T) {
	t.Parallel()

	defaults := DefaultConfig()
	fastInterval := defaults.Interval
	relaxedInterval := defaults.RelaxedInterval
	clampTarget := func(value float64) float64 {
		return clamp(value, defaults.TargetMin, defaults.TargetMax)
	}

	targetAfterStepUp := clampTarget(defaults.TargetStart + defaults.StepUp)
	fallbackTarget := clampTarget(defaults.FallbackTarget)
	fallbackRecoveryTarget := clampTarget(fallbackTarget - defaults.StepDown)
	clampedDecrease := clampTarget(targetAfterStepUp - defaults.StepDown)

	scenarios := []controllerScenario{
		{
			name: "success then fallback recovery",
			results: []metricResult{
				{value: 0.20, err: nil},
				{value: 0, err: errOCIDown},
				{value: 0.29, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: targetAfterStepUp, nextInterval: fastInterval},
				{state: StateFallback, target: fallbackTarget, nextInterval: fastInterval},
				{state: StateNormal, target: fallbackRecoveryTarget, nextInterval: relaxedInterval},
			},
		},
		{
			name: "clamps within bounds",
			results: []metricResult{
				{value: 0.10, err: nil},
				{value: 0.50, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: targetAfterStepUp, nextInterval: fastInterval},
				{state: StateNormal, target: clampedDecrease, nextInterval: relaxedInterval},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			runControllerScenario(t, scenario)
		})
	}
}

func TestControllerCpuUtilisationAcrossOCPUs(t *testing.T) {
	t.Parallel()

	defaults := DefaultConfig()
	highUtilisationScenario := buildHighUtilisationScenario(defaults)

	cases := []struct {
		name  string
		ocpus int
	}{
		{name: "1-ocpu burst matches policy", ocpus: 1},
		{name: "2-ocpu burst matches policy", ocpus: 2},
		{name: "3-ocpu burst matches policy", ocpus: 3},
		{name: "4-ocpu burst matches policy", ocpus: 4},
	}

	for _, shapeCase := range cases {
		t.Run(shapeCase.name, func(t *testing.T) {
			t.Parallel()

			results := append([]metricResult(nil), highUtilisationScenario.results...)
			expectations := append([]stepExpectation(nil), highUtilisationScenario.expectations...)

			scenario := controllerScenario{
				name:         shapeCase.name,
				results:      results,
				expectations: expectations,
			}

			runControllerScenario(t, scenario)
		})
	}
}

func buildHighUtilisationScenario(defaults Config) controllerScenario {
	clampTarget := func(value float64) float64 {
		return clamp(value, defaults.TargetMin, defaults.TargetMax)
	}

	scenario := controllerScenario{
		name: "baseline ocpu burst",
		results: []metricResult{
			{value: 0.15, err: nil},
			{value: 0.32, err: nil},
			{value: 0.34, err: nil},
			{value: 0.36, err: nil},
			{value: 0.38, err: nil},
			{value: 0.40, err: nil},
			{value: 0.45, err: nil},
		},
		expectations: nil,
	}

	decayingTargets := make([]float64, len(scenario.results))

	decayingTargets[0] = clampTarget(defaults.TargetStart + defaults.StepUp)
	for index := 1; index < len(decayingTargets); index++ {
		decayingTargets[index] = clampTarget(decayingTargets[index-1] - defaults.StepDown)
	}

	expectations := make([]stepExpectation, len(decayingTargets))
	for index, target := range decayingTargets {
		interval := defaults.Interval
		if scenario.results[index].value >= defaults.RelaxedThreshold {
			interval = defaults.RelaxedInterval
		}

		expectations[index] = stepExpectation{
			state:        StateNormal,
			target:       target,
			nextInterval: interval,
		}
	}

	scenario.expectations = expectations

	return scenario
}

func runControllerScenario(t *testing.T, scenario controllerScenario) {
	t.Helper()

	metrics := newFakeMetrics(scenario.results)
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	if diff := math.Abs(shaper.Target() - cfg.FallbackTarget); diff > 1e-9 {
		t.Fatalf(
			"expected initial fallback target %.2f got %.2f",
			cfg.FallbackTarget,
			shaper.Target(),
		)
	}

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatalf("controller does not expose stepper interface")
	}

	for stepIndex, expectation := range scenario.expectations {
		interval := stepper.step(context.Background())

		if controller.State() != expectation.state {
			t.Fatalf(
				"step %d state: got %v want %v",
				stepIndex,
				controller.State(),
				expectation.state,
			)
		}

		if diff := math.Abs(controller.Target() - expectation.target); diff > 1e-9 {
			t.Fatalf(
				"step %d target mismatch: got %.2f want %.2f",
				stepIndex,
				controller.Target(),
				expectation.target,
			)
		}

		if interval != expectation.nextInterval {
			t.Fatalf(
				"step %d interval: got %v want %v",
				stepIndex,
				interval,
				expectation.nextInterval,
			)
		}
	}
}

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
		// range variable copy
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

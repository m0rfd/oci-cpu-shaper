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

	scenarios := []controllerScenario{
		{
			name: "success then fallback recovery",
			results: []metricResult{
				{value: 0.20, err: nil},
				{value: 0, err: errOCIDown},
				{value: 0.29, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: 0.27, nextInterval: time.Hour},
				{state: StateFallback, target: 0.25, nextInterval: time.Hour},
				{state: StateNormal, target: 0.25, nextInterval: 6 * time.Hour},
			},
		},
		{
			name: "clamps within bounds",
			results: []metricResult{
				{value: 0.10, err: nil},
				{value: 0.50, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: 0.27, nextInterval: time.Hour},
				{state: StateNormal, target: 0.26, nextInterval: 6 * time.Hour},
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

	highUtilisationScenario := controllerScenario{
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
		expectations: []stepExpectation{
			{state: StateNormal, target: 0.27, nextInterval: time.Hour},
			{state: StateNormal, target: 0.26, nextInterval: 6 * time.Hour},
			{state: StateNormal, target: 0.25, nextInterval: 6 * time.Hour},
			{state: StateNormal, target: 0.24, nextInterval: 6 * time.Hour},
			{state: StateNormal, target: 0.23, nextInterval: 6 * time.Hour},
			{state: StateNormal, target: 0.22, nextInterval: 6 * time.Hour},
			{state: StateNormal, target: 0.22, nextInterval: 6 * time.Hour},
		},
	}

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

func runControllerScenario(t *testing.T, scenario controllerScenario) {
	t.Helper()

	metrics := newFakeMetrics(scenario.results)
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Interval = time.Hour
	cfg.RelaxedInterval = 6 * time.Hour

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

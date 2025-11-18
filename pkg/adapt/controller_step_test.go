// Package adapt exercises the controller step transitions to ensure duty-cycle
// scheduling keeps matching the production policy table.
//
//nolint:testpackage,godoclint // Tests need internal helpers and separate coverage notes per file.
package adapt

import (
	"context"
	"errors"
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

type controllerStepper interface {
	step(ctx context.Context) time.Duration
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
				{state: StateNormal, target: 0.27, nextInterval: 6 * time.Hour},
				{state: StateFallback, target: 0.25, nextInterval: time.Hour},
				{state: StateNormal, target: 0.25, nextInterval: time.Hour},
			},
		},
		{
			name: "clamps within bounds",
			results: []metricResult{
				{value: 0.10, err: nil},
				{value: 0.50, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: 0.27, nextInterval: 6 * time.Hour},
				{state: StateNormal, target: 0.26, nextInterval: time.Hour},
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
			{state: StateNormal, target: 0.27, nextInterval: 6 * time.Hour},
			{state: StateNormal, target: 0.26, nextInterval: time.Hour},
			{state: StateNormal, target: 0.25, nextInterval: time.Hour},
			{state: StateNormal, target: 0.24, nextInterval: time.Hour},
			{state: StateNormal, target: 0.23, nextInterval: time.Hour},
			{state: StateNormal, target: 0.22, nextInterval: time.Hour},
			{state: StateNormal, target: 0.22, nextInterval: time.Hour},
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

func TestAdaptiveControllerRecordsLastError(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0, err: errOCIDown}})
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatalf("controller does not expose stepper interface")
	}

	stepper.step(context.Background())

	lastErr := controller.LastError()
	if !errors.Is(lastErr, errOCIDown) {
		t.Fatalf("expected last error to be errOCIDown, got %v", lastErr)
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

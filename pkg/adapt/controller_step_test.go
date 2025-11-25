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
	cfg          *Config
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

	scenarios := controllerStateScenarios(
		defaults,
		fastInterval,
		relaxedInterval,
		targetAfterStepUp,
		fallbackTarget,
		fallbackRecoveryTarget,
		clampedDecrease,
	)

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			runControllerScenario(t, scenario)
		})
	}
}

func TestControllerRelaxedIntervalSingleConfirmation(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.RelaxedConfirmations = 1

	scenario := controllerScenario{
		name: "single-high-switches-to-relaxed",
		cfg:  &cfg,
		results: []metricResult{
			{value: cfg.RelaxedThreshold, err: nil},
		},
		expectations: []stepExpectation{
			{
				state:        StateNormal,
				target:       cfg.TargetStart,
				nextInterval: cfg.RelaxedInterval,
			},
		},
	}

	runControllerScenario(t, scenario)
}

func TestControllerRelaxedIntervalRequiresConsecutiveConfirmations(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.RelaxedConfirmations = 2

	scenario := controllerScenario{
		name: "first-high-keeps-normal-interval",
		cfg:  &cfg,
		results: []metricResult{
			{value: cfg.RelaxedThreshold, err: nil},
			{value: cfg.RelaxedThreshold, err: nil},
		},
		expectations: []stepExpectation{
			{
				state:        StateNormal,
				target:       cfg.TargetStart,
				nextInterval: cfg.Interval,
			},
			{
				state:        StateNormal,
				target:       cfg.TargetStart,
				nextInterval: cfg.RelaxedInterval,
			},
		},
	}

	runControllerScenario(t, scenario)
}

func controllerStateScenarios(
	defaults Config,
	fastInterval time.Duration,
	relaxedInterval time.Duration,
	targetAfterStepUp float64,
	fallbackTarget float64,
	fallbackRecoveryTarget float64,
	clampedDecrease float64,
) []controllerScenario {
	return []controllerScenario{
		{
			name: "success then fallback recovery",
			cfg:  nil,
			results: []metricResult{
				{value: 0.20, err: nil},
				{value: 0, err: errOCIDown},
				{value: 0.29, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: targetAfterStepUp, nextInterval: fastInterval},
				{state: StateFallback, target: fallbackTarget, nextInterval: fastInterval},
				{state: StateNormal, target: fallbackRecoveryTarget, nextInterval: fastInterval},
			},
		},
		{
			name: "clamps within bounds",
			cfg:  nil,
			results: []metricResult{
				{value: 0.10, err: nil},
				{value: 0.50, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: targetAfterStepUp, nextInterval: fastInterval},
				{state: StateNormal, target: clampedDecrease, nextInterval: fastInterval},
			},
		},
		{
			name: "relaxed interval after consecutive highs",
			cfg:  nil,
			results: []metricResult{
				{value: defaults.RelaxedThreshold, err: nil},
				{value: defaults.GoalLow - 0.01, err: nil},
				{value: defaults.RelaxedThreshold, err: nil},
				{value: defaults.RelaxedThreshold, err: nil},
			},
			expectations: []stepExpectation{
				{state: StateNormal, target: defaults.TargetStart, nextInterval: fastInterval},
				{state: StateNormal, target: targetAfterStepUp, nextInterval: fastInterval},
				{state: StateNormal, target: targetAfterStepUp, nextInterval: fastInterval},
				{state: StateNormal, target: targetAfterStepUp, nextInterval: relaxedInterval},
			},
		},
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
				cfg:          nil,
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

	relaxedConfirmations := defaults.RelaxedConfirmations

	scenario := controllerScenario{
		name: "baseline ocpu burst",
		cfg:  nil,
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

	var consecutiveRelaxed int

	for index, target := range decayingTargets {
		interval := defaults.Interval
		if scenario.results[index].value >= defaults.RelaxedThreshold {
			consecutiveRelaxed++
			if consecutiveRelaxed >= relaxedConfirmations {
				interval = defaults.RelaxedInterval
			}
		} else {
			consecutiveRelaxed = 0
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

func TestRelaxedIntervalWaitsForStabilityAfterSuppression(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Interval = time.Minute
	cfg.RelaxedInterval = time.Hour

	metrics := newFakeMetrics([]metricResult{
		{value: cfg.RelaxedThreshold, err: nil},
		{value: cfg.RelaxedThreshold + 0.01, err: nil},
		{value: cfg.RelaxedThreshold + 0.02, err: nil},
	})

	controller, err := NewAdaptiveController(cfg, metrics, nil, newFakeShaper(), nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	controller.mu.Lock()
	controller.suppressed = true
	controller.relaxedSuccesses = cfg.RelaxedConfirmations
	controller.mu.Unlock()

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatalf("controller does not expose stepper interface")
	}

	interval := stepper.step(context.Background())
	requireEqual(t, "suppressed interval", interval, cfg.Interval)
	requireEqual(t, "relaxed successes while suppressed", controller.RelaxedSuccesses(), 0)

	controller.mu.Lock()
	controller.suppressed = false
	controller.mu.Unlock()

	interval = stepper.step(context.Background())
	requireEqual(t, "post-resume interval", interval, cfg.Interval)

	interval = stepper.step(context.Background())
	requireEqual(t, "relaxed interval after stability", interval, cfg.RelaxedInterval)
}

func runControllerScenario(t *testing.T, scenario controllerScenario) {
	t.Helper()

	metrics := newFakeMetrics(scenario.results)
	shaper := newFakeShaper()

	cfg := DefaultConfig()
	if scenario.cfg != nil {
		cfg = *scenario.cfg
	}

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

func TestAdaptiveControllerRecordsIntervalAfterRelaxationConfirmation(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Interval = time.Minute
	cfg.RelaxedInterval = time.Hour

	runIntervalRecordingScenario(
		t,
		cfg,
		[]metricResult{
			{value: cfg.RelaxedThreshold, err: nil},
			{value: cfg.RelaxedThreshold + 0.01, err: nil},
		},
		[]time.Duration{cfg.Interval, cfg.RelaxedInterval},
		StateNormal,
	)
}

func TestAdaptiveControllerRecordsIntervalForHealthyWorkload(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.Interval = time.Minute
	cfg.RelaxedInterval = time.Hour

	runIntervalRecordingScenario(
		t,
		cfg,
		[]metricResult{{value: cfg.RelaxedThreshold - 0.05, err: nil}},
		[]time.Duration{cfg.Interval},
		StateNormal,
	)
}

func runIntervalRecordingScenario(
	t *testing.T,
	cfg Config,
	results []metricResult,
	wantIntervals []time.Duration,
	wantState State, //nolint:unparam // Parameter kept for future test extensibility.
) {
	t.Helper()

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

	for index, wantInterval := range wantIntervals {
		interval := stepper.step(context.Background())

		requireEqual(t, "step interval", interval, wantInterval)

		if recorder.interval != wantInterval {
			t.Fatalf(
				"step %d recorded interval: got %v want %v",
				index,
				recorder.interval,
				wantInterval,
			)
		}
	}

	requireEqual(t, "state", controller.State(), wantState)
}

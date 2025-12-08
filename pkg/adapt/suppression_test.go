// Package adapt groups suppression-related observation tests to highlight the
// coverage around estimator backpressure and high-load clamping logic.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"context"
	"math"
	"testing"
)

func TestConsumeEstimatorSuppression(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.SuppressThreshold = 0.8
	cfg.SuppressResume = 0.5

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservation(controller, 0, 0.9, nil)
	feedObservation(controller, 1, 0.95, nil)

	if controller.State() != StateSuppressed {
		t.Fatalf("expected suppressed state after high utilisation, got %v", controller.State())
	}

	if controller.Target() != 0 {
		t.Fatalf(
			"expected target to drop to zero during suppression, got %.2f",
			controller.Target(),
		)
	}

	for i := 0; i < 6 && controller.State() == StateSuppressed; i++ {
		feedObservation(controller, int64(2+i), 0.10, nil)
	}

	if controller.State() != StateFallback {
		t.Fatalf("expected controller to resume fallback after cooling, got %v", controller.State())
	}

	if diff := math.Abs(controller.Target() - cfg.FallbackTarget); diff > 1e-9 {
		t.Fatalf(
			"expected fallback target %.2f after suppression, got %.2f",
			cfg.FallbackTarget,
			controller.Target(),
		)
	}

	if len(shaper.Calls()) < 2 {
		t.Fatalf(
			"expected shaper to be called for suppression transitions, got %d calls",
			len(shaper.Calls()),
		)
	}

	if len(shaper.HostSignals()) == 0 {
		t.Fatal("expected shaper to observe host load samples")
	}
}

func TestConsumeEstimatorRunnableSuppression(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.SuppressThreshold = 0
	cfg.SuppressResume = 0
	cfg.SuppressRunnableThreshold = 1.1
	cfg.SuppressRunnableResume = 0.5

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservationWithRunnable(controller, 0, 0.1, 1.2, nil)

	if controller.State() != StateSuppressed {
		t.Fatalf("expected runnable surge to suppress controller, got %v", controller.State())
	}

	if controller.Target() != 0 {
		t.Fatalf("expected suppressed target to drop to zero, got %.2f", controller.Target())
	}

	feedObservationWithRunnable(controller, 1, 0.1, 0.2, nil)

	if controller.State() != StateFallback {
		t.Fatalf(
			"expected controller to resume after runnable cooldown, got %v",
			controller.State(),
		)
	}
}

func TestSuppressionGuardBypassesHostSmoothing(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.SuppressSmoothingSamples = 10

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservation(controller, 0, 0.3, nil)

	feedObservation(controller, 1, 0.95, nil)

	if controller.State() != StateSuppressed {
		t.Fatalf("expected utilisation spike to suppress controller, got %v", controller.State())
	}

	if math.Abs(controller.hostLoad-0.95) > 1e-9 {
		t.Fatalf("expected host load to jump to spike, got %.3f", controller.hostLoad)
	}

	feedObservation(controller, 2, 0.10, nil)

	if controller.State() != StateSuppressed {
		t.Fatalf(
			"expected suppression to persist while host load decays, got %v",
			controller.State(),
		)
	}

	for i := 0; i < 4 && controller.State() == StateSuppressed; i++ {
		feedObservation(controller, int64(3+i), 0.1, nil)
	}

	if controller.State() != StateFallback {
		t.Fatalf("expected controller to resume after cooldown, got %v", controller.State())
	}

	if diff := math.Abs(controller.Target() - cfg.FallbackTarget); diff > 1e-9 {
		t.Fatalf(
			"expected fallback target %.2f after cooldown, got %.2f",
			cfg.FallbackTarget,
			controller.Target(),
		)
	}
}

func TestGuardedSuppressionOverridesMetrics(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	controller.mu.Lock()
	controller.hostLoad = cfg.SuppressThreshold * 0.25
	controller.hostRunnable = cfg.SuppressRunnableThreshold * 0.25
	controller.applyTargetLocked(cfg.FallbackTarget * 1.1)
	previouslySuppressed := controller.transitionSuppressionLocked(true)
	controller.applySuppressionTargetsLocked(previouslySuppressed)
	controller.updateEffectiveStateLocked()
	target := controller.target
	controller.mu.Unlock()

	requireConditionf(t, !previouslySuppressed, "guarded suppression should report previous false")
	assertState(t, controller, StateSuppressed, "guard path should force suppression")
	assertTargetZero(t, controller, "guarded suppression should zero target")

	if shaper.Target() != target {
		t.Fatalf(
			"expected shaper target %.2f to match controller target %.2f",
			shaper.Target(),
			target,
		)
	}
}

func TestTransitionSuppressionGuardedSwitch(t *testing.T) {
	t.Parallel()

	controller, shaper, cfg := newSuppressionHarness(t)

	controller.mu.Lock()
	controller.suppressed = false
	controller.hostLoad = cfg.SuppressThreshold * 0.25
	controller.hostRunnable = cfg.SuppressRunnableThreshold * 0.25
	controller.applyTargetLocked(cfg.FallbackTarget * 1.2)
	previously := controller.transitionSuppressionLocked(true)
	controller.applySuppressionTargetsLocked(previously)
	controller.updateEffectiveStateLocked()
	target := controller.target
	controller.mu.Unlock()

	requireConditionf(t, !previously, "guarded suppression should return previous state")
	assertState(t, controller, StateSuppressed, "guard path should force suppression")
	assertTargetZero(t, controller, "guard path should zero target")

	if shaper.Target() != target {
		t.Fatalf(
			"expected shaper target %.2f to match controller target %.2f",
			shaper.Target(),
			target,
		)
	}
}

func TestTransitionSuppressionUtilisation(t *testing.T) {
	t.Parallel()

	controller, shaper, cfg := newSuppressionHarness(t)

	controller.mu.Lock()
	controller.suppressed = false
	controller.hostLoad = cfg.SuppressThreshold + 0.05
	controller.hostRunnable = cfg.SuppressRunnableThreshold * 0.5
	controller.applyTargetLocked(cfg.TargetMax)
	utilisationSuppressed := controller.transitionSuppressionLocked(false)
	controller.applySuppressionTargetsLocked(utilisationSuppressed)
	controller.updateEffectiveStateLocked()
	controller.mu.Unlock()

	requireConditionf(
		t,
		controller.shouldSuppressForUtilisation(),
		"host load should trigger utilisation suppression",
	)
	requireConditionf(
		t,
		!utilisationSuppressed,
		"utilisation suppression should report previous false",
	)
	assertState(t, controller, StateSuppressed, "high utilisation should suppress controller")
	assertTargetZero(t, controller, "utilisation suppression should zero target")

	if shaper.Target() != 0 {
		t.Fatalf("expected shaper to receive suppressed target, got %.2f", shaper.Target())
	}
}

func TestTransitionSuppressionRunnables(t *testing.T) {
	t.Parallel()

	controller, shaper, cfg := newSuppressionHarness(t)

	controller.mu.Lock()
	controller.suppressed = false
	controller.hostLoad = cfg.SuppressThreshold * 0.5
	controller.hostRunnable = cfg.SuppressRunnableThreshold + 0.2
	controller.applyTargetLocked(cfg.TargetMax)
	runnableSuppressed := controller.transitionSuppressionLocked(false)
	controller.applySuppressionTargetsLocked(runnableSuppressed)
	controller.updateEffectiveStateLocked()
	controller.mu.Unlock()

	requireConditionf(
		t,
		controller.shouldSuppressForRunnables(),
		"runnable surge should trigger suppression",
	)
	requireConditionf(t, !runnableSuppressed, "runnable suppression should report previous false")
	assertState(t, controller, StateSuppressed, "runnable surge should suppress controller")
	assertTargetZero(t, controller, "runnable suppression should zero target")

	if shaper.Target() != 0 {
		t.Fatalf("expected shaper to receive suppressed target, got %.2f", shaper.Target())
	}
}

func TestTransitionSuppressionResumeRestoresTarget(t *testing.T) {
	t.Parallel()

	controller, shaper, cfg := newSuppressionHarness(t)

	controller.mu.Lock()
	controller.suppressed = true
	controller.target = 0
	controller.desired = cfg.TargetMax * 1.5
	controller.hostLoad = cfg.SuppressResume * 0.8
	controller.hostRunnable = cfg.SuppressRunnableResume * 0.8
	previously := controller.transitionSuppressionLocked(false)
	controller.applySuppressionTargetsLocked(previously)
	controller.updateEffectiveStateLocked()
	restored := controller.target
	controller.mu.Unlock()

	requireConditionf(t, previously, "resume should report previous suppression")
	assertState(t, controller, StateFallback, "cooled metrics should resume controller")

	expected := clamp(cfg.TargetMax*1.5, cfg.TargetMin, cfg.TargetMax)
	if restored != expected {
		t.Fatalf("expected restored target %.2f after resume, got %.2f", expected, restored)
	}

	if shaper.Target() != expected {
		t.Fatalf(
			"expected shaper to receive restored target %.2f, got %.2f",
			expected,
			shaper.Target(),
		)
	}
}

func TestConsumeEstimatorHandlesErrors(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservation(controller, 0, 0, errEstimatorObservation)

	if controller.LastEstimatorError() == nil {
		t.Fatal("expected estimator error to be recorded")
	}

	if controller.State() != StateFallback {
		t.Fatalf(
			"expected fallback state to remain after estimator error, got %v",
			controller.State(),
		)
	}

	if diff := math.Abs(controller.Target() - cfg.FallbackTarget); diff > 1e-9 {
		t.Fatalf(
			"expected fallback target to remain %.2f after estimator error, got %.2f",
			cfg.FallbackTarget,
			controller.Target(),
		)
	}
}

func TestSuppressionTransitionsRestoreDesiredTarget(t *testing.T) {
	t.Parallel()

	controller, shaper, cfg := newSuppressionHarness(t)
	initialTarget := controller.Target()

	feedObservationWithRunnable(controller, 0, cfg.SuppressThreshold+0.05, 0, nil)
	assertSuppressedTargets(
		t,
		controller,
		initialTarget,
		"guarded spike should suppress controller",
	)

	stepOnce(t, controller)

	requireConditionf(
		t,
		controller.desired > initialTarget,
		"desired should rise while suppressed, got %.2f",
		controller.desired,
	)
	assertTargetZero(t, controller, "target should stay zero while suppressed")

	feedObservationWithRunnable(
		controller,
		1,
		cfg.SuppressResume*0.9,
		cfg.SuppressRunnableThreshold+0.1,
		nil,
	)
	assertState(t, controller, StateSuppressed, "runnable spike should keep suppression")

	feedObservationWithRunnable(
		controller,
		2,
		cfg.SuppressResume*0.9,
		cfg.SuppressRunnableResume*0.9,
		nil,
	)
	assertState(t, controller, StateNormal, "cooldown should restore controller")

	restored := assertRestoredTarget(t, controller)

	assertShaperCallSequence(t, shaper, []float64{initialTarget, 0, 0, restored})
}

func TestSuppressionResumesAfterBothMetricsCool(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	controller.mu.Lock()
	controller.suppressed = true
	controller.target = 0
	controller.desired = cfg.FallbackTarget * 1.2
	controller.hostLoad = cfg.SuppressResume * 0.8
	controller.hostRunnable = cfg.SuppressRunnableResume * 0.8
	previouslySuppressed := controller.transitionSuppressionLocked(false)
	controller.applySuppressionTargetsLocked(previouslySuppressed)
	controller.updateEffectiveStateLocked()
	restored := controller.target
	controller.mu.Unlock()

	requireConditionf(
		t,
		previouslySuppressed,
		"suppression exit should report previous suppression",
	)
	assertState(t, controller, StateFallback, "cooled metrics should resume controller")

	if restored != controller.desired {
		t.Fatalf(
			"expected suppression exit to restore desired target %.2f, got %.2f",
			controller.desired,
			restored,
		)
	}

	if shaper.Target() != restored {
		t.Fatalf(
			"expected shaper to receive restored target %.2f, got %.2f",
			restored,
			shaper.Target(),
		)
	}
}

func newSuppressionHarness(t *testing.T) (*AdaptiveController, *fakeShaper, Config) {
	t.Helper()

	metrics := newFakeMetrics([]metricResult{{value: 0.19, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.SuppressSmoothingSamples = 1

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	requireConditionf(t, err == nil, "NewAdaptiveController: %v", err)

	return controller, shaper, cfg
}

func stepOnce(t *testing.T, controller *AdaptiveController) {
	t.Helper()

	stepper, ok := any(controller).(controllerStepper)
	requireConditionf(t, ok, "controller does not expose stepper interface")

	stepper.step(context.Background())
}

func assertSuppressedTargets(
	t *testing.T,
	controller *AdaptiveController,
	expectedDesired float64,
	reason string,
) {
	t.Helper()

	assertState(t, controller, StateSuppressed, reason)
	assertTargetZero(t, controller, reason)

	requireConditionf(
		t,
		controller.desired == expectedDesired,
		"desired should stay fallback, got %.2f vs %.2f",
		expectedDesired,
		controller.desired,
	)
}

func assertState(t *testing.T, controller *AdaptiveController, expected State, reason string) {
	t.Helper()

	requireConditionf(t, controller.State() == expected, "%s, got %v", reason, controller.State())
}

func assertTargetZero(t *testing.T, controller *AdaptiveController, reason string) {
	t.Helper()

	requireConditionf(t, controller.Target() == 0, "%s: target %.2f", reason, controller.Target())
}

func assertRestoredTarget(t *testing.T, controller *AdaptiveController) float64 {
	t.Helper()

	requireConditionf(
		t,
		controller.Target() != 0,
		"restored target should be non-zero, got %.2f",
		controller.Target(),
	)
	requireConditionf(
		t,
		controller.Target() == controller.desired,
		"restored target should match desired %.2f, got %.2f",
		controller.desired,
		controller.Target(),
	)

	return controller.Target()
}

func assertShaperCallSequence(t *testing.T, shaper *fakeShaper, expected []float64) {
	t.Helper()

	requireConditionf(
		t,
		len(shaper.Calls()) == len(expected),
		"expected %d shaper calls, got %d",
		len(expected),
		len(shaper.Calls()),
	)

	for index, expectedValue := range expected {
		requireConditionf(
			t,
			math.Abs(shaper.Calls()[index]-expectedValue) <= 1e-9,
			"shaper call %d should be %.2f, got %.2f",
			index,
			expectedValue,
			shaper.Calls()[index],
		)
	}
}

func requireConditionf(t *testing.T, condition bool, format string, args ...any) {
	t.Helper()

	if condition {
		return
	}

	t.Fatalf(format, args...)
}

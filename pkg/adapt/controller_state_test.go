// Package adapt isolates the controller state accessors to keep the getter
// expectations documented alongside the implementation.
//
//nolint:testpackage,godoclint // Tests exercise private helpers and provide coverage docstrings.
package adapt

import (
	"context"
	"testing"
)

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
	if lastErr == nil {
		t.Fatalf("expected error to be recorded")
	}

	if lastErr.Error() != errOCIDown.Error() {
		t.Fatalf("expected last error to be errOCIDown, got %v", lastErr)
	}
}

func TestAdaptiveControllerStateAccessors(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}})
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.Mode = enforceMode

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatalf("controller does not expose stepper interface")
	}

	stepper.step(context.Background())

	if controller.State() != StateNormal {
		t.Fatalf("expected state to transition to normal, got %v", controller.State())
	}

	if controller.Target() == 0 {
		t.Fatalf("expected target to be updated")
	}

	if controller.LastP95() == 0 {
		t.Fatalf("expected last p95 to be recorded")
	}

	feedObservation(controller, 0, 0.5, errEstimatorObservation)

	if controller.LastEstimatorError() == nil {
		t.Fatalf("expected estimator error to be recorded")
	}

	feedObservation(controller, 0, 0.6, nil)

	if controller.LastEstimatorError() != nil {
		t.Fatalf("expected estimator error to be cleared")
	}

	if controller.Mode() != enforceMode {
		t.Fatalf("expected mode to be enforce, got %q", controller.Mode())
	}
}

func TestClampEnforcesBounds(t *testing.T) {
	t.Parallel()

	if got := clamp(-0.5, 0, 1); got != 0 {
		t.Fatalf("expected clamp to return lower bound, got %f", got)
	}

	if got := clamp(1.5, 0, 1); got != 1 {
		t.Fatalf("expected clamp to return upper bound, got %f", got)
	}

	if got := clamp(0.5, 0, 1); got != 0.5 {
		t.Fatalf("expected clamp to preserve value within bounds, got %f", got)
	}
}

func TestApplySuppressionTargetsLockedRestoresClampedStart(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}})
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	controller.mu.Lock()
	controller.desired = 0
	controller.suppressed = false
	controller.cfg.TargetMin = 0.15
	controller.cfg.TargetMax = 0.25
	controller.cfg.TargetStart = 0.30
	controller.applySuppressionTargetsLocked(true)
	target := controller.target
	controller.mu.Unlock()

	if target != controller.cfg.TargetMax {
		t.Fatalf(
			"expected target to clamp to max %.2f after suppression, got %.2f",
			controller.cfg.TargetMax,
			target,
		)
	}

	if shaper.Target() != controller.cfg.TargetMax {
		t.Fatalf(
			"expected shaper to receive clamped target %.2f, got %.2f",
			controller.cfg.TargetMax,
			shaper.Target(),
		)
	}
}

func TestApplySuppressionTargetsLockedMaintainsZeroWhenSuppressed(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}})
	shaper := newFakeShaper()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	controller.mu.Lock()
	controller.applyTargetLocked(0.2)
	controller.suppressed = true
	controller.applySuppressionTargetsLocked(false)
	target := controller.target
	controller.mu.Unlock()

	if target != 0 {
		t.Fatalf("expected suppression to force target to zero, got %.2f", target)
	}

	if shaper.Target() != 0 {
		t.Fatalf(
			"expected shaper to receive zero target while suppressed, got %.2f",
			shaper.Target(),
		)
	}
}

func TestRelaxedSuccessesGetter(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	metrics := newFakeMetrics([]metricResult{{value: 0.30, err: nil}})
	shaper := newFakeShaper()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	// Initially should be 0
	if controller.RelaxedSuccesses() != 0 {
		t.Fatalf("expected initial relaxedSuccesses = 0, got %d", controller.RelaxedSuccesses())
	}

	// After one high sample, should be 1
	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatal("controller does not expose stepper interface")
	}

	_ = stepper.step(context.Background())

	if controller.RelaxedSuccesses() != 1 {
		t.Fatalf(
			"expected relaxedSuccesses = 1 after first high sample, got %d",
			controller.RelaxedSuccesses(),
		)
	}
}

func TestRelaxedSuccessesResetOnDrop(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.RelaxedThreshold = 0.26

	metrics := newFakeMetrics([]metricResult{
		{value: 0.30, err: nil}, // High
		{value: 0.24, err: nil}, // Drop below threshold
	})
	shaper := newFakeShaper()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	stepper, ok := any(controller).(controllerStepper)
	if !ok {
		t.Fatal("controller does not expose stepper interface")
	}

	// First step: counter should increment
	_ = stepper.step(context.Background())

	if controller.RelaxedSuccesses() != 1 {
		t.Fatalf("expected relaxedSuccesses = 1, got %d", controller.RelaxedSuccesses())
	}

	// Second step: counter should reset to 0
	_ = stepper.step(context.Background())

	if controller.RelaxedSuccesses() != 0 {
		t.Fatalf("expected relaxedSuccesses = 0 after drop, got %d", controller.RelaxedSuccesses())
	}
}

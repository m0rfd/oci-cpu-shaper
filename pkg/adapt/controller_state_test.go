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

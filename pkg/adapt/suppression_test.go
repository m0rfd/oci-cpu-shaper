// Package adapt groups suppression-related observation tests to highlight the
// coverage around estimator backpressure and high-load clamping logic.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"math"
	"testing"
)

func TestConsumeEstimatorSuppression(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}})
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

	if len(shaper.calls) < 2 {
		t.Fatalf(
			"expected shaper to be called for suppression transitions, got %d calls",
			len(shaper.calls),
		)
	}

	if len(shaper.hostLoads) == 0 {
		t.Fatal("expected shaper to observe host load samples")
	}
}

func TestConsumeEstimatorHandlesErrors(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}})
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

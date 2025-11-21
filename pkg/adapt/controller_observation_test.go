// Package adapt focuses observation handling coverage to keep estimator
// responses isolated from the ticker loop and step transitions.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import "testing"

func TestHandleObservationClearsEstimatorError(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}})
	shaper := newFakeShaper()
	cfg := DefaultConfig()
	cfg.SuppressThreshold = 0

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, nil)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservation(controller, 0, 0, errEstimatorObservation)

	if controller.LastEstimatorError() == nil {
		t.Fatal("expected estimator error to be recorded")
	}

	feedObservation(controller, 1, 0.5, nil)

	if controller.LastEstimatorError() != nil {
		t.Fatal("expected estimator error to be cleared after successful observation")
	}

	if len(shaper.hostLoads) == 0 {
		t.Fatal("expected host load to be forwarded to the shaper")
	}
}

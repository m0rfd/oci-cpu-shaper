// Package adapt focuses observation handling coverage to keep estimator
// responses isolated from the ticker loop and step transitions.
//
//nolint:testpackage,godoclint // Tests need internal helpers and per-file coverage documentation.
package adapt

import (
	"math"
	"testing"
)

func TestHandleObservationClearsEstimatorError(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
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

	if len(shaper.HostSignals()) == 0 {
		t.Fatal("expected host load to be forwarded to the shaper")
	}
}

func TestHandleObservationGuardSuppression(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	recorder := newStubMetricsRecorder()
	cfg := DefaultConfig()

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservationWithRunnable(controller, 0, 0.3, cfg.SuppressRunnableThreshold*1.5, nil)

	if controller.State() != StateSuppressed {
		t.Fatalf("expected guard-triggered suppression, got %s", controller.State())
	}

	if shaper.TargetValue() != 0 {
		t.Fatalf("expected guard to drop target to zero, got %.2f", shaper.TargetValue())
	}

	signals := shaper.HostSignals()

	lastSignal := signals[len(signals)-1]
	if lastSignal.runnable == 0 {
		t.Fatal("expected runnable signal to be forwarded to shaper")
	}

	if recorder.hostCalls == 0 {
		t.Fatal("expected recorder to receive host observation")
	}
}

func TestHandleObservationGuardSuppressionDisabled(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	recorder := newStubMetricsRecorder()
	cfg := DefaultConfig()
	cfg.SuppressThreshold = 0
	cfg.SuppressResume = 0
	cfg.SuppressRunnableThreshold = 0
	cfg.SuppressRunnableResume = 0

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservationWithRunnable(controller, 0, 0.9, 2, nil)

	controller.mu.Lock()
	controller.handleGuardedSuppressionLocked(true)
	controller.mu.Unlock()

	if !controller.suppressed {
		t.Fatal(
			"expected controller to be suppressed when guard trips even with suppression disabled",
		)
	}

	if controller.Target() != 0 {
		t.Fatalf("expected target to drop to zero, got %.2f", controller.Target())
	}

	if shaper.TargetValue() != 0 {
		t.Fatalf("expected shaper to receive zero target, got %.2f", shaper.TargetValue())
	}

	if recorder.state != StateSuppressed.String() {
		t.Fatalf("expected recorder to be notified of suppressed state, got %s", recorder.state)
	}

	if recorder.target != 0 {
		t.Fatalf("expected recorder to be notified of zero target, got %.2f", recorder.target)
	}

	if recorder.hostCalls == 0 {
		t.Fatal("expected recorder to receive host observation")
	}
}

func TestHandleObservationGuardSuppressionTransitions(t *testing.T) {
	t.Parallel()

	metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
	shaper := newFakeShaper()
	recorder := newStubMetricsRecorder()
	cfg := DefaultConfig()
	cfg.SuppressSmoothingSamples = 1

	controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
	if err != nil {
		t.Fatalf("NewAdaptiveController: %v", err)
	}

	feedObservationWithRunnable(
		controller,
		0,
		cfg.SuppressThreshold*1.1,
		cfg.SuppressRunnableThreshold*1.5,
		nil,
	)
	requireEqual(t, "state", controller.State(), StateSuppressed)
	requireFloatApprox(t, "target", controller.Target(), 0)

	feedObservationWithRunnable(
		controller,
		1,
		cfg.SuppressResume*0.5,
		cfg.SuppressRunnableResume*0.5,
		nil,
	)
	requireEqual(t, "state", controller.State(), StateFallback)
	requireFloatApprox(t, "restored target", controller.Target(), cfg.FallbackTarget)
	requireTrue(t, "recorder target calls", recorder.targetCalls >= 3)
	requireTrue(t, "recorder state calls", recorder.stateCalls >= 3)
}

func TestHandleObservationNormalizesRunnable(t *testing.T) {
	t.Parallel()

	testCases := map[string]float64{
		"nan":      math.NaN(),
		"pos-inf":  math.Inf(1),
		"neg-inf":  math.Inf(-1),
		"negative": -1,
	}

	for name, runnable := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			metrics := newFakeMetrics([]metricResult{{value: 0.25, err: nil}}) //nolint:exhaustruct
			shaper := newFakeShaper()
			recorder := newStubMetricsRecorder()
			cfg := DefaultConfig()

			controller, err := NewAdaptiveController(cfg, metrics, nil, shaper, recorder)
			if err != nil {
				t.Fatalf("NewAdaptiveController: %v", err)
			}

			feedObservationWithRunnable(controller, 0, 0.5, runnable, nil)

			if controller.hostRunnable != 0 {
				t.Fatalf(
					"expected controller runnable to be normalized, got %.2f",
					controller.hostRunnable,
				)
			}

			signals := shaper.HostSignals()
			if len(signals) != 1 {
				t.Fatalf("expected shaper to receive 1 observation, got %d", len(signals))
			}

			lastSignal := signals[0]
			if lastSignal.runnable != 0 {
				t.Fatalf("expected shaper runnable to be normalized, got %.2f", lastSignal.runnable)
			}

			if recorder.hostCalls != 1 {
				t.Fatalf("expected recorder to observe host CPU once, got %d", recorder.hostCalls)
			}

			if recorder.host != 0.5 {
				t.Fatalf(
					"expected recorder host utilisation to match observation, got %.2f",
					recorder.host,
				)
			}
		})
	}
}
